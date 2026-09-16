variable "project_id" {
  description = "GCP project that owns the disposable contract resources."
  type        = string
}

variable "bucket_name" {
  description = "Globally unique name for the disposable contract bucket."
  type        = string
}

variable "location" {
  description = "GCS location for the disposable contract bucket."
  type        = string
  default     = "US"
}

variable "github_owner" {
  description = "GitHub repository owner allowed to use federation."
  type        = string
  default     = "reeveops"
}

variable "github_repository" {
  description = "GitHub repository allowed to use federation."
  type        = string
  default     = "reeve-test"
}

variable "github_owner_id" {
  description = "Immutable GitHub owner ID allowed to use federation."
  type        = string
  default     = "310228695"
}

variable "github_repository_id" {
  description = "Immutable GitHub repository ID allowed to use federation."
  type        = string
  default     = "1231457834"
}

variable "github_ref" {
  description = "Exact Git ref allowed to use federation."
  type        = string
  default     = "refs/heads/master"
}

variable "service_account_id" {
  description = "Service account used by the cloud contract workflow."
  type        = string
  default     = "reeve-e2e"
}

variable "workload_identity_pool_id" {
  description = "Workload Identity Pool ID for GitHub Actions."
  type        = string
  default     = "reeve-e2e"
}

variable "workload_identity_provider_id" {
  description = "Workload Identity Pool provider ID for GitHub Actions."
  type        = string
  default     = "github"
}
