variable "aws_region" { type = string; default = "eu-west-2" }
variable "project_name" { type = string; default = "core-platform" }
variable "environment" { type = string; default = "dev" }
variable "cloudflare_api_token" { type = string; sensitive = true; default = null }
