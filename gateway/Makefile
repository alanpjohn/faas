export DOCKER_CLI_EXPERIMENTAL=enabled

PLATFORM?="linux/amd64,linux/arm/v7,linux/arm64"

TAG?=dev
OWNER?=alanpjohn
NAME=gateway-tracing

.PHONY: buildx-local
buildx-local:
	@echo $(OWNER)/$(NAME):$(TAG) \
	&& docker buildx create --use --name=multiarch --node multiarch \
	&& docker buildx build \
		--progress=plain \
		--platform linux/amd64 \
		--output "type=docker,push=false" \
		--tag $(OWNER)/$(NAME):$(TAG) .

.PHONY: buildx-push
buildx-push:
	@echo $(OWNER)/$(NAME):$(TAG) \
	&& docker buildx create --use --name=multiarch --node multiarch \
	&& docker buildx build \
		--progress=plain \
		--platform linux/amd64 \
		--output "type=image,push=true" \
		--tag $(OWNER)/$(NAME):$(TAG) .

.PHONY: buildx-push-all
buildx-push-all:
	@echo $(OWNER)/$(NAME):$(TAG) \
	&& docker buildx create --use --name=multiarch --node multiarch \
	&& docker buildx build \
		--progress=plain \
		--platform $(PLATFORM) \
		--output "type=image,push=true" \
		--tag $(OWNER)/$(NAME):$(TAG) .
