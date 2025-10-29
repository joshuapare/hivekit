module github.com/joshuapare/hivekit/cmd/hivectl

go 1.25.3

require (
	github.com/joshuapare/hivekit v0.0.0
	github.com/spf13/cobra v1.10.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/text v0.30.0 // indirect
)

replace github.com/joshuapare/hivekit => ../../

replace github.com/joshuapare/hivekit/bindings => ../../bindings
