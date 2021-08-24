package main

import (
	"context"

	"github.com/rb3ckers/trafficmirror/cmd"
	"github.com/rs/zerolog/log"
)

func main() {
	ctx := log.Logger.WithContext(context.Background())
	cmd.Execute(ctx)
}
