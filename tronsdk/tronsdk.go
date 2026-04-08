package tronsdk

import (
	"net"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewClient() (*client.GrpcClient, error) {

	fn := func(addr string, d time.Duration) (conn net.Conn, err error) {
		return net.DialTimeout("tcp", addr, d)
	}

	c := client.NewGrpcClientWithTimeout(url, 30*time.Second)

	dailOptions := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(50*1024*1024),
			grpc.MaxCallSendMsgSize(50*1024*1024),
		),
		grpc.WithDialer(fn),
	}

	err := c.Start(dailOptions...)
	if err != nil {
		return nil, err
	}
	return c, nil
}
