build:
	CGO_ENABLED=0 go build -o wg2 ./cmd/wg2
	docker build . -t shynome/libp2p-wg2
push:
	docker push shynome/libp2p-wg2
