export TEST_IMAGE ?= docker.io/library/alpine:3

test:
	docker pull $(TEST_IMAGE)
	ginkgo run -cover -race -v .
