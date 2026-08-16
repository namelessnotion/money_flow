#!/bin/bash
protoc --proto_path=./proto --go_out=./go/rpc/ --go_opt=paths=source_relative --twrip_go_out=./go/gen/proto --twrip_go_opt=paths=source_relative proto/holder/holder.proto
protoc --proto_path=./proto --ruby_out=./ruby/rpc/ --ruby_opt=paths=source_relative --twrip_ruby_out=./go/gen/proto --twrip_ruby_opt=paths=source_relative proto/holder/holder.proto

grpc_tools_ruby_protoc -I ./proto --ruby_out=ruby/gen/proto --grpc_out=ruby/gen/proto ./proto/holder/holder.proto
