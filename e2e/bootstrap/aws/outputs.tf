output "github_role_arn" {
  description = "Value for the E2E_AWS_ROLE_ARN repository variable."
  value       = aws_iam_role.github.arn
}

output "region" {
  description = "Value for the E2E_AWS_REGION repository variable."
  value       = var.region
}

output "bucket_name" {
  description = "Value for the E2E_S3_BUCKET repository variable."
  value       = aws_s3_bucket.contract.id
}
