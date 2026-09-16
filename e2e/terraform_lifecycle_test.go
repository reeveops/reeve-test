//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTerraformLifecycle(t *testing.T) {
	t.Parallel()
	s := newHCLSuiteInReport(t, "terraform", "terraform", "Terraform", *terraformFlag)

	s.newHead("terraform-create")
	s.preview("terraform-create-preview", counts{Add: 1})
	s.github.approve("", "", "")
	s.apply("terraform-create-apply", false)
	s.require(len(s.resources()) == 1, "Terraform create did not persist one resource")

	s.newHead("terraform-update")
	source := filepath.Join(s.module, "main.tf")
	s.write(source, []byte(strings.ReplaceAll(string(s.read(source)), `"initial"`, `"updated"`)), 0600)
	s.preview("terraform-update-preview", counts{Change: 1})
	s.github.approve("", "", "")
	s.apply("terraform-update-apply", false)
	s.preview("terraform-update-converged", counts{})

	var state struct {
		Resources []struct {
			Instances []struct {
				Attributes struct {
					Input struct{ Value string }
				}
			}
		}
	}
	s.readJSON(filepath.Join(s.module, "terraform.tfstate"), &state)
	s.require(len(state.Resources) == 1 && len(state.Resources[0].Instances) == 1,
		"Terraform update did not persist one resource instance")
	s.require(state.Resources[0].Instances[0].Attributes.Input.Value == "updated",
		"Terraform resource input was not updated")

	s.newHead("terraform-replace")
	s.write(source, []byte(`terraform {}

resource "terraform_data" "item" {
  input            = "replaced"
  triggers_replace = "force-v1"
}
`), 0600)
	s.preview("terraform-replace-preview", counts{Replace: 1})
	s.github.approve("", "", "")
	s.apply("terraform-replace-apply", false)

	s.newHead("terraform-delete")
	s.write(source, []byte("terraform {}\n"), 0600)
	s.preview("terraform-delete-preview", counts{Delete: 1})
	s.github.approve("", "", "")
	s.apply("terraform-delete-apply", false)
	s.require(len(s.resources()) == 0, "Terraform delete left managed resources")
	s.preview("terraform-delete-converged", counts{})

	s.require(len(s.results) == 10, "Expected 10 Terraform command scenarios, got %d", len(s.results))
}
