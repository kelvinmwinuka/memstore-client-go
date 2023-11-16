package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
)

func main() {
	conf := GetConfig()

	var conn net.Conn
	var err error

	if !conf.TLS {
		fmt.Println("Starting src in TCP mode...")

		conn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", conf.Addr, conf.Port))
		if err != nil {
			panic(err)
		}
	} else {
		// Dial TLS
		fmt.Println("Starting src in TLS mode...")

		f, err := os.Open(conf.Cert)

		if err != nil {
			panic(err)
		}

		cert, err := io.ReadAll(bufio.NewReader(f))

		if err != nil {
			panic(err)
		}

		rootCAs := x509.NewCertPool()

		ok := rootCAs.AppendCertsFromPEM(cert)
		if !ok {
			panic("Failed to parse certificate")
		}

		conn, err = tls.Dial("tcp", fmt.Sprintf("%s:%d", conf.Addr, conf.Port), &tls.Config{
			RootCAs: rootCAs,
		})

		if err != nil {
			panic(fmt.Sprintf("Handshake Error: %s", err.Error()))
		}
	}

	defer conn.Close()

	done := make(chan struct{})

	connRW := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	stdioRW := bufio.NewReadWriter(bufio.NewReader(os.Stdin), bufio.NewWriter(os.Stdout))

	go func() {
		for {
			stdioRW.Write([]byte("\n> "))
			stdioRW.Flush()

			if in, err := stdioRW.ReadBytes(byte('\n')); err != nil {
				fmt.Println(err)
			} else {
				in := bytes.TrimSpace(in)

				// Check for quit command
				if bytes.Equal(bytes.ToLower(in), []byte("quit")) {
					break
				}

				// Serialize command and send to connection
				encoded, err := Encode(string(in))

				if err != nil {
					fmt.Println(err)
					continue
				}

				connRW.Write([]byte(encoded))
				connRW.Flush()

				// Read response from server
				message, err := ReadMessage(connRW)

				if err != nil && err == io.EOF {
					fmt.Println(err)
					break
				} else if err != nil {
					fmt.Println(err)
				}

				decoded, err := Decode(message)

				if err != nil {
					fmt.Println(err)
					continue
				}

				fmt.Println(decoded)

				if decoded[0] == "SUBSCRIBE_OK" {
					// If we're subscribed to a channel, listen for messages from the channel
					func() {
						for {
							var message string

							if msg, err := ReadMessage(connRW); err != nil {
								if err == io.EOF {
									return
								}
								fmt.Println(err)
								continue
							} else {
								message = msg
							}

							if decoded, err := Decode(message); err != nil {
								fmt.Println(err)
								continue
							} else {
								connRW.Write([]byte("+ACK\r\n\n"))
								connRW.Flush()

								fmt.Println(decoded)
							}
						}
					}()
				}
			}
		}
		done <- struct{}{}
	}()

	<-done
}
