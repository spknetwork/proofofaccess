package connections

import (
	"fmt"
	"net"
	"time"
)

func ConnectToNode(address string) (net.Conn, error) {
	for {
		conn, err := net.Dial("tcp", address)
		if err != nil {
			fmt.Printf("Error connecting to node: %v. Retrying in 5 seconds...\n", err)
			time.Sleep(5 * time.Second)
			continue
		}
		return conn, nil
	}
}

func Listen(listenPort int) (net.Listener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", listenPort))
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		return nil, err
	}

	fmt.Printf("Listening on port %d...\n", listenPort)
	return listener, nil
}

func HandleConnection(conn net.Conn) {
	defer conn.Close()
	//scanner := bufio.NewScanner(conn)
	//messaging.ReceiveMessage(conn, scanner)
}
