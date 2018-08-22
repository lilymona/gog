all: message.proto
	protoc --proto_path=${GOPATH}/src:${GOPATH}/src/github.com/gogo/protobuf:. --gogo_out=. message.proto
