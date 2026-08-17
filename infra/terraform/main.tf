provider "aws" {
  region = var.aws_region
}

resource "aws_s3_bucket" "platform_files" {
  bucket_prefix = "${var.project_name}-${var.environment}-files-"
  force_destroy = false
  tags = {
    Project     = var.project_name
    Environment = var.environment
  }
}

resource "aws_s3_bucket_versioning" "platform_files" {
  bucket = aws_s3_bucket.platform_files.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "platform_files" {
  bucket = aws_s3_bucket.platform_files.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# EKS, managed PostgreSQL-compatible data services, Kafka, cache and multi-region
# resources should be added as environment modules. They are intentionally not
# auto-created by this starter to avoid accidental high cloud spend.
