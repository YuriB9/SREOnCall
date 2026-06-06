module github.com/sre-oncall/scheduling

go 1.26.4
toolchain go1.26.4

require (
	github.com/go-chi/chi/v5 v5.1.0
	github.com/sre-oncall/pkg v0.0.0
)

replace github.com/sre-oncall/pkg => ../../pkg
