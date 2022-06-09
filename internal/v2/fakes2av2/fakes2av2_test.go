package fakes2av2

import (
	"net"
	"log"
	"time"
	"sync"
	"context"
	"testing"
	"google.golang.org/grpc"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/testing/protocmp"

	s2av2pb "github.com/google/s2a-go/internal/proto/v2/s2a_go_proto"
	commonpb "github.com/google/s2a-go/internal/proto/v2/common_go_proto"
)

const (
	defaultTimeout = 10.0 * time.Second
)

func startFakeS2Av2Server(wg *sync.WaitGroup) (address string, stop func(), err error) {
	// Pick unused port.
	listener, err := net.Listen("tcp", ":0")
	address = listener.Addr().String()
	if err != nil {
		log.Fatalf("failed to listen on address %s: %v", listener.Addr().String(), err)
	}
	s := grpc.NewServer()
	log.Printf("Server: started gRPC Fake S2Av2 Server on address: %s", listener.Addr().String())
	s2av2pb.RegisterS2AServiceServer(s, &Server{})
	go func() {
		wg.Done()
		if err := s.Serve(listener); err != nil {
			log.Printf("failed to serve: %v", err)
		}
	}()
	return address, func() { s.Stop()}, nil
}

func TestSetUpSession(t *testing.T) {
	for _, tc := range []struct {
		description		string
		request			*s2av2pb.SessionReq
		expectedResponse	*s2av2pb.SessionResp
	}{
		{
			description: "Get TLS config for client.",
			request: &s2av2pb.SessionReq {
				AuthenticationMechanisms: []*s2av2pb.AuthenticationMechanism {
					{
						MechanismOneof: &s2av2pb.AuthenticationMechanism_Token{"token"},
					},
				},
				ReqOneof: &s2av2pb.SessionReq_GetTlsConfigurationReq {
					&s2av2pb.GetTlsConfigurationReq {
						ConnectionSide: commonpb.ConnectionSide_CONNECTION_SIDE_CLIENT,
					},
				},
			},
			expectedResponse: &s2av2pb.SessionResp {
				Status: &s2av2pb.Status {
					Code: 0,
				},
				RespOneof: &s2av2pb.SessionResp_GetTlsConfigurationResp {
					GetTlsConfigurationResp: &s2av2pb.GetTlsConfigurationResp {
						TlsConfiguration: &s2av2pb.GetTlsConfigurationResp_ClientTlsConfiguration_ {
							&s2av2pb.GetTlsConfigurationResp_ClientTlsConfiguration {
								CertificateChain: []string{
									string(clientCert),
								},
								MinTlsVersion: commonpb.TLSVersion_TLS_VERSION_1_3,
								MaxTlsVersion: commonpb.TLSVersion_TLS_VERSION_1_3,
								HandshakeCiphersuites: []commonpb.HandshakeCiphersuite{},
								RecordCiphersuites: []commonpb.RecordCiphersuite {
									commonpb.RecordCiphersuite_RECORD_CIPHERSUITE_AES_128_GCM_SHA256,
									commonpb.RecordCiphersuite_RECORD_CIPHERSUITE_AES_256_GCM_SHA384,
									commonpb.RecordCiphersuite_RECORD_CIPHERSUITE_CHACHA20_POLY1305_SHA256,
								},
							},
						},
					},
				},
			},
		},
		{
			description: "Get TLS config for server.",
			request: &s2av2pb.SessionReq {
				AuthenticationMechanisms: []*s2av2pb.AuthenticationMechanism {
					{
						MechanismOneof: &s2av2pb.AuthenticationMechanism_Token{"token"},
					},
				},
				ReqOneof: &s2av2pb.SessionReq_GetTlsConfigurationReq {
					&s2av2pb.GetTlsConfigurationReq {
						ConnectionSide: commonpb.ConnectionSide_CONNECTION_SIDE_SERVER,
					},
				},
			},
			expectedResponse: &s2av2pb.SessionResp {
				Status: &s2av2pb.Status {
					Code: 0,
				},
				RespOneof: &s2av2pb.SessionResp_GetTlsConfigurationResp {
					GetTlsConfigurationResp: &s2av2pb.GetTlsConfigurationResp {
						TlsConfiguration: &s2av2pb.GetTlsConfigurationResp_ServerTlsConfiguration_{
							&s2av2pb.GetTlsConfigurationResp_ServerTlsConfiguration{
								CertificateChain: []string{
									string(serverCert),
								},
								MinTlsVersion: commonpb.TLSVersion_TLS_VERSION_1_3,
								MaxTlsVersion: commonpb.TLSVersion_TLS_VERSION_1_3,
								HandshakeCiphersuites: []commonpb.HandshakeCiphersuite{},
								RecordCiphersuites: []commonpb.RecordCiphersuite {
									commonpb.RecordCiphersuite_RECORD_CIPHERSUITE_AES_128_GCM_SHA256,
									commonpb.RecordCiphersuite_RECORD_CIPHERSUITE_AES_256_GCM_SHA384,
									commonpb.RecordCiphersuite_RECORD_CIPHERSUITE_CHACHA20_POLY1305_SHA256,
								},
								TlsResumptionEnabled: false,
								RequestClientCertificate: s2av2pb.GetTlsConfigurationResp_ServerTlsConfiguration_REQUEST_AND_VERIFY,
								MaxOverheadOfTicketAead: 0,
							},
						},
					},
				},
			},
		},
	}{
		t.Run(tc.description, func(t *testing.T) {
			// Start up Fake S2Av2 Server.
			var wg sync.WaitGroup
			wg.Add(1)
			address, stop, err := startFakeS2Av2Server(&wg)
			wg.Wait()

			// Create stream to server.
			opts := []grpc.DialOption{
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithReturnConnectionError(),
				grpc.WithBlock(),
			}
			conn, err := grpc.Dial(address, opts...)
			if err != nil {
				t.Fatalf("Client: failed to connect: %v", err)
			}
			defer conn.Close()
			c := s2av2pb.NewS2AServiceClient(conn)
			log.Printf("Client: connected to: %s", address)
			ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			defer cancel()

			// Setup bidrectional streaming session.
			callOpts := []grpc.CallOption{}
			cstream, err := c.SetUpSession(ctx, callOpts...)
			if err != nil  {
				t.Fatalf("Client: failed to setup bidirectional streaming RPC session: %v", err)
			}
			log.Printf("Client: set up bidirectional streaming RPC session.")

			// Send request.
			if err := cstream.Send(tc.request); err != nil {
				t.Fatalf("Client: failed to send SessionReq: %v", err)
			}
			log.Printf("Client: sent SessionReq")

			// Get the Response.
			resp, err := cstream.Recv()
			if err != nil {
				t.Fatalf("Client: failed to receive SessionResp: %v", err)
			}
			log.Printf("Client: recieved SessionResp")
			if diff := cmp.Diff(resp, tc.expectedResponse, protocmp.Transform()); diff != "" {
				t.Errorf("cstream.Recv() returned incorrect SessionResp, (-want +got):\n%s", diff)
			}
			log.Printf("resp matches tc.expectedResponse")
			stop()
		})
	}
}
