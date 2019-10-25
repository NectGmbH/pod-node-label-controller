TAG=v0.1.0
IMAGE=kavatech/pod-node-label-controller:$(TAG)

export GOOS=linux

all: build docker-build

build:
	go build

docker-build:
	docker build -t $(IMAGE) .

docker-push:
	docker push $(IMAGE)