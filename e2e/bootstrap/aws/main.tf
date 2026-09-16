terraform {
  required_version = ">= 1.8.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      ManagedBy  = "OpenTofu"
      Purpose    = "reeve-e2e"
      Repository = local.repository
    }
  }
}

data "tls_certificate" "github" {
  count = var.create_github_oidc_provider ? 1 : 0
  url   = "https://token.actions.githubusercontent.com"
}

locals {
  repository = "${var.github_owner}/${var.github_repository}"
  github_subject = var.use_immutable_github_subject ? (
    "repo:${var.github_owner}@${var.github_owner_id}/${var.github_repository}@${var.github_repository_id}:ref:${var.github_ref}"
  ) : "repo:${local.repository}:ref:${var.github_ref}"
  oidc_provider_arn = var.create_github_oidc_provider ? (
    aws_iam_openid_connect_provider.github[0].arn
  ) : var.github_oidc_provider_arn
}

resource "aws_s3_bucket" "contract" {
  bucket        = var.bucket_name
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "contract" {
  bucket = aws_s3_bucket.contract.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "contract" {
  bucket = aws_s3_bucket.contract.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "contract" {
  bucket = aws_s3_bucket.contract.id

  depends_on = [aws_s3_bucket_versioning.contract]

  rule {
    id     = "expire-abandoned-contract-data"
    status = "Enabled"

    filter {
      prefix = "reeve-test/"
    }

    expiration {
      days = 7
    }

    noncurrent_version_expiration {
      noncurrent_days = 7
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 1
    }
  }
}

resource "aws_iam_openid_connect_provider" "github" {
  count = var.create_github_oidc_provider ? 1 : 0

  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.github[0].certificates[length(data.tls_certificate.github[0].certificates) - 1].sha1_fingerprint]
}

data "aws_iam_policy_document" "github_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [local.oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = [local.github_subject]
    }
  }
}

resource "aws_iam_role" "github" {
  name               = var.role_name
  assume_role_policy = data.aws_iam_policy_document.github_assume_role.json
}

data "aws_iam_policy_document" "bucket" {
  statement {
    sid       = "ListContractPrefix"
    effect    = "Allow"
    actions   = ["s3:ListBucket"]
    resources = [aws_s3_bucket.contract.arn]

    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["reeve-test/*"]
    }
  }

  statement {
    sid    = "ManageContractObjects"
    effect = "Allow"
    actions = [
      "s3:DeleteObject",
      "s3:GetObject",
      "s3:PutObject",
    ]
    resources = ["${aws_s3_bucket.contract.arn}/reeve-test/*"]
  }
}

resource "aws_iam_role_policy" "bucket" {
  name   = "reeve-e2e-bucket"
  role   = aws_iam_role.github.id
  policy = data.aws_iam_policy_document.bucket.json
}
