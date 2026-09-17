terraform {
  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

variable "length" {
  type        = number
  default     = 3
  description = "Number of words in the generated pet name."
}

resource "random_pet" "pet" {
  length    = var.length
  separator = "-"
}

output "pet_name" {
  value = random_pet.pet.id
}
