build:
	mkdir -p ./buildtarget
	CGO_ENABLED=0 go build -o ./buildtarget/imagesync-loader .
	
clean:
	rm -rf ./buildtarget/*
