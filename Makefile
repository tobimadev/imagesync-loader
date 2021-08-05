#PROJ = imageplug-downloader
#NOW = $(shell date -u +"%y%m%d%H%M")
#GIT_COMMIT = $(shell git rev-parse --short HEAD)

#VER = $(NOW)-$(GIT_COMMIT)

#DIFF_STATUS = $(shell git status --porcelain)
#ifneq ($(DIFF_STATUS),)
#GIT_COMMIT := "localbuild"
#endif

#IMAGE_NAME = $(PROJ):$(NOW).$(GIT_COMMIT)
##BUILDER_IMAGE_NAME = $(PROJ)-builder:$(NOW)
##LDFLAGS = -extldflags -static -w

build:
	mkdir -p ./buildtarget
	CGO_ENABLED=0 go build -o ./buildtarget/imagesync-loader ./cmd/imagesync-loader
	
clean:
	rm -rf ./buildtarget/*

