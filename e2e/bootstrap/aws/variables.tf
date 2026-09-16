variable "region" {
  description = "AWS region for the disposable contract bucket."
  type        = string
}

variable "bucket_name" {
  description = "Globally unique name for the disposable contract bucket."
  type        = string
}

variable "github_owner" {
  description = "GitHub repository owner allowed to assume the role."
  type        = string
  default     = "reeveops"
}

variable "github_repository" {
  description = "GitHub repository allowed to assume the role."
  type        = string
  default     = "reeve-test"
}

variable "use_immutable_github_subject" {
  description = "Use GitHub's owner-ID and repository-ID OIDC subject format."
  type        = bool
  default     = true
}

variable "github_owner_id" {
  description = "Immutable GitHub owner ID included in the OIDC subject."
  type        = string
  default     = "310228695"
}

variable "github_repository_id" {
  description = "Immutable GitHub repository ID included in the OIDC subject."
  type        = string
  default     = "1231457834"
}

variable "github_ref" {
  description = "Exact Git ref allowed to assume the role."
  type        = string
  default     = "refs/heads/master"
}

variable "role_name" {
  description = "IAM role assumed by the cloud contract workflow."
  type        = string
  default     = "reeve-e2e-github"
}

variable "create_github_oidc_provider" {
  description = "Create GitHub's IAM OIDC provider in this account."
  type        = bool
  default     = true
}

variable "github_oidc_provider_arn" {
  description = "Existing GitHub IAM OIDC provider ARN when creation is disabled."
  type        = string
  default     = null

  validation {
    condition = var.create_github_oidc_provider || (
      var.github_oidc_provider_arn != null && var.github_oidc_provider_arn != ""
    )
    error_message = "github_oidc_provider_arn is required when create_github_oidc_provider is false."
  }
}
