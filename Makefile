all:
	go build -o build/serve serve.go
	go build -o build/sacar sacar.go

install:	all
	cp -vf build/serve ${HOME}/bin
	cp -vf build/sacar ${HOME}/bin
