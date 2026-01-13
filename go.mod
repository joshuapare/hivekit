module github.com/joshuapare/hivekit

go 1.25.3

replace github.com/joshuapare/hivekit/bindings => ./bindings

require github.com/joshuapare/hivekit/bindings v0.0.0-00010101000000-000000000000

require (
	github.com/stretchr/testify v1.11.1
	golang.org/x/sys v0.37.0
	golang.org/x/text v0.30.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
