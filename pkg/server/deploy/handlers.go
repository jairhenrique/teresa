package deploy

import (
	"bytes"
	"io"
	"time"

	"google.golang.org/grpc"

	"github.com/luizalabs/teresa/pkg/goutil"
	dpb "github.com/luizalabs/teresa/pkg/protobuf/deploy"
	"github.com/luizalabs/teresa/pkg/server/database"
)

const (
	keepAliveMessage = "\u200B" // Zero width space
)

type Options struct {
	KeepAliveTimeout     time.Duration `split_words:"true" default:"30s"`
	RevisionHistoryLimit int           `split_words:"true" default:"5"`
	SlugBuilderImage     string        `split_words:"true" default:"luizalabs/slugbuilder:v2.5.0"`
	SlugRunnerImage      string        `split_words:"true" default:"luizalabs/slugrunner:v2.4.0"`
	BuildLimitCPU        string        `split_words:"true" default:"800m"`
	BuildLimitMemory     string        `split_words:"true" default:"1Gi"`
}

type Service struct {
	ops     Operations
	options *Options
}

func (s *Service) Make(stream dpb.Deploy_MakeServer) error {
	var appName, description string
	content := new(bytes.Buffer)

	ctx := stream.Context()
	u := ctx.Value("user").(*database.User)

	for {
		in, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if info := in.GetInfo(); info != nil {
			appName = info.App
			description = info.Description
		}
		if data := in.GetFile(); data != nil {
			content.Write(data.Chunk)
		}
	}

	rs := bytes.NewReader(content.Bytes())
	rc, err := s.ops.Deploy(u, appName, rs, description, s.options)
	if err != nil {
		return err
	}
	defer rc.Close()

	deployMsgs := goutil.ChannelFromReader(rc, true)
	var msg string

	for {
		select {
		case <-time.After(s.options.KeepAliveTimeout):
			msg = keepAliveMessage
		case m, ok := <-deployMsgs:
			if !ok {
				return nil
			}
			msg = m
		}

		if err := stream.Send(&dpb.DeployResponse{Text: msg}); err != nil {
			return err
		}
	}
}

func (s *Service) RegisterService(grpcServer *grpc.Server) {
	dpb.RegisterDeployServer(grpcServer, s)
}

func NewService(ops Operations, options *Options) *Service {
	return &Service{ops: ops, options: options}
}
