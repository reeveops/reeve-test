output "workload_identity_provider" {
  description = "Value for the E2E_GCP_WORKLOAD_IDENTITY_PROVIDER repository variable."
  value       = google_iam_workload_identity_pool_provider.github.name
}

output "service_account" {
  description = "Value for the E2E_GCP_SERVICE_ACCOUNT repository variable."
  value       = google_service_account.github.email
}

output "bucket_name" {
  description = "Value for the E2E_GCS_BUCKET repository variable."
  value       = google_storage_bucket.contract.name
}
