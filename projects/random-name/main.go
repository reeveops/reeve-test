package main

import (
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		length := cfg.GetInt("length")
		if length == 0 {
			length = 2
		}

		pet, err := random.NewRandomPet(ctx, "pet", &random.RandomPetArgs{
			Length:    pulumi.Int(length),
			Separator: pulumi.String("-"),
		})
		if err != nil {
			return err
		}
		ctx.Export("petName", pet.ID())
		return nil
	})
}
