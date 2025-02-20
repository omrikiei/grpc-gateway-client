package test

import (
	"context"
	"encoding/base64"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/omrikiei/grpc-gateway-client/internal/test/gen/testv1"
	"github.com/omrikiei/grpc-gateway-client/internal/test/server"
	"github.com/omrikiei/grpc-gateway-client/pkg/grpc/gateway"
)

type ClientTestSuite struct {
	suite.Suite

	l       *bufconn.Listener
	grpcSrv *grpc.Server
	gwSrv   *httptest.Server
	client  testv1.TestServiceGatewayClient
}

func (s *ClientTestSuite) SetupTest() {
	s.l = bufconn.Listen(256 * 1024)
	s.grpcSrv = grpc.NewServer()
	testv1.RegisterTestServiceServer(s.grpcSrv, server.NewTestServer())
	go func() {
		_ = s.grpcSrv.Serve(s.l)
	}()

	cc, err := grpc.Dial("",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return s.l.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	s.Require().NoError(err)

	marshaller := &runtime.JSONPb{}
	sseMarshaller := gateway.NewEventStreamMarshaller(marshaller)
	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption("application/json", marshaller),
		runtime.WithMarshalerOption("text/event-stream", sseMarshaller),
	)
	s.Require().NoError(testv1.RegisterTestServiceHandler(context.TODO(), mux, cc))
	s.gwSrv = httptest.NewServer(mux)
	s.client = testv1.NewTestServiceGatewayClient(gateway.NewClient(s.gwSrv.URL))
}

func (s *ClientTestSuite) TestSendInvitation() {
	_, err := s.client.SendInvitation(context.TODO(), &testv1.SendInvitationRequest{
		Email: "abc@def.com",
	})
	s.Require().NoError(err)
}

func (s *ClientTestSuite) TestTrackInvitation() {
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second)
	defer cancel()

	resCh, _, err := s.client.TrackInvitation(context.TODO(), &testv1.TrackInvitationRequest{
		Id: base64.StdEncoding.EncodeToString([]byte("abc@def.com")),
	})
	s.Require().NoError(err)

	got := make([]testv1.EventType, 0)

	for {
		select {
		case <-ctx.Done():
			s.Require().NotEmpty(got)
			return
		case res, ok := <-resCh:
			if !ok {
				s.Require().NotEmpty(got)
				return
			}
			got = append(got, res.Type)
		}
	}
}

func (s *ClientTestSuite) TestListInvitations() {
	req := &testv1.ListInvitationsRequest{
		Query: &testv1.ListInvitationsQuery{
			Labels: map[string]string{
				"abc": "def",
				"xyz": "123",
			},
		},
	}
	res, err := s.client.ListInvitations(context.TODO(), req)
	s.Require().NoError(err)
	s.Require().Len(res.GetInvitations(), 1)
	actual := res.GetInvitations()[0]
	s.Require().NotEmpty(actual.GetId())
	s.Require().Equal(req.GetQuery().GetLabels(), actual.GetLabels())
}

func (s *ClientTestSuite) TearDownTest() {
	s.gwSrv.Close()
	s.grpcSrv.Stop()
	_ = s.l.Close()
}

func TestClientTestSuite(t *testing.T) {
	suite.Run(t, &ClientTestSuite{})
}
