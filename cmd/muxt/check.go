package main

import (
	"fmt"
	"io"
	"log"

	"github.com/crhntr/muxt/internal/configuration"
	"github.com/crhntr/muxt/internal/muxt"
)

func checkCommand(workingDirectory string, args []string, stderr io.Writer) error {
	config, err := configuration.NewRoutesFileConfiguration(args, stderr)
	if err != nil {
		return err
	}
	if err := muxt.CheckTemplates(workingDirectory, log.New(stderr, "", 0), config); err != nil {
		return fmt.Errorf("fail: %s", err)
	}
	return nil
}
