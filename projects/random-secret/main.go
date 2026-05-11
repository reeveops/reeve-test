package main

import (
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		length := cfg.GetInt("passwordLength")
		if length == 0 {
			length = 16
		}

		pw, err := random.NewRandomPassword(ctx, "password", &random.RandomPasswordArgs{
			Length:  pulumi.Int(length),
			Special: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}
		// Mark as a secret output so reeve's redactor sees the [secret]
		// marker in the plan and replaces it with [redacted].
		ctx.Export("password", pulumi.ToSecret(pw.Result))
		return nil
	})
}
