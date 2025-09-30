// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"log"

	"github.com/melg8/swarm/internal/swarm/connection"
)

func connectAndAuthenticate() error {
	connector, err := connection.ServerConnector("127.0.0.1:2106")
	if err != nil {
		return fmt.Errorf("failed to create server connector: %w", err)
	}

	conn, err := connector.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer conn.Close()

	if err := connection.AuthentificateConn(conn); err != nil {
		return fmt.Errorf("failed to authentificate connection: %w", err)
	}

	log.Println("Connection authentificated")
	return nil
}

type MyError string

func (e MyError) Error() string {
	return string(e)
}

func Test() error {
	return MyError("Retruning error without any packages used")
}

func main() {
	log.Println("Starting connect bot...")
	if err := connectAndAuthenticate(); err != nil {
		log.Fatal(err)
	}
}
