module github.com/luisfurquim/wprana/example

go 1.21

require (
	github.com/luisfurquim/goose v0.0.0
	github.com/luisfurquim/wprana v0.0.0
)

replace (
	github.com/luisfurquim/goose => ../../goose
	github.com/luisfurquim/wprana => ../
)
