GOCMD=go
GOBUILD=$(GOCMD) build -ldflags="-s -w"
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get

all:clean timespotter

timespotter:
	$(GOBUILD) timespotter.go dnssrv.go file.go 

clean:
	$(GOCLEAN)

