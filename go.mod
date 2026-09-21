// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

module github.com/melg8/swarm

go 1.23.2

// The formatting source of truth: the tree is gofmt-spaces clean
// under the Go 1.26 gofmt (the struct field comment layout changed
// in the 1.25 line). Every `go` invocation inside the module
// switches to this toolchain, so `task fmt:check` agrees with the
// committed tree on every host (the deb go1.24 GOROOT included).
toolchain go1.26.8

require (
	github.com/klauspost/compress v1.18.0
	github.com/sergi/go-diff v1.4.0
	github.com/stretchr/testify v1.12.1
	golang.org/x/crypto v0.28.0
	golang.org/x/text v0.19.0
)

require go.yaml.in/yaml/v3 v3.0.5 // indirect
