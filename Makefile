build:
	CGO_ENABLED=0 go build -o wg2 ./cmd/wg2
	docker build . -t shynome/libp2p-wg2
push:
	docker push shynome/libp2p-wg2
run: build
	docker run --rm -ti --name wg2 --privileged --net host shynome/libp2p-wg2 -key 1