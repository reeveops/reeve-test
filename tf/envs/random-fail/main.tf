# Deliberately failing module: the plan is a clean +1 create
# (terraform_data is builtin, no provider download), but the create-time
# provisioner exits 1, so only apply fails. Mirrors projects/random-fail.
# The empty terraform block marks this directory as a root module for
# reeve's discovery.
terraform {}
resource "terraform_data" "always_fails" {
  provisioner "local-exec" {
    command = "echo 'simulated deploy failure' >&2 && exit 1"
  }
}
