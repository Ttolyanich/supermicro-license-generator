module supermicro-license-generator

go 1.21

require (
	github.com/pkg/errors v0.9.1
	github.com/rs/zerolog v1.31.0
	github.com/spf13/cobra v1.8.0
	github.com/zsrv/supermicro-product-key v0.0.0
	golang.org/x/sys v0.15.0
)

replace github.com/zsrv/supermicro-product-key => ./upstream
