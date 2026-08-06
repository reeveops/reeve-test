package main

import (
	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Preview plans this as an ordinary +1 create; the create command
		// itself exits nonzero, so only apply fails.
		cmd, err := local.NewCommand(ctx, "always-fails", &local.CommandArgs{
			Create: pulumi.String("echo 'simulated deploy failure' >&2 && exit 1"),
		})
		if err != nil {
			return err
		}
		ctx.Export("stdout", cmd.Stdout)
		return nil
	})
}
