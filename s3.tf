# module "s3-bucket" {
#   source  = "terraform-aws-modules/s3-bucket/aws"
#   version = "4.2.2"
#     bucket        = "ffnotifier-build"
#   force_destroy = true
# }

# module "s3-bucket_object" {
#   source  = "terraform-aws-modules/s3-bucket/aws//modules/object"
#   version = "4.2.2"

#   bucket = module.s3_bucket.s3_bucket_id
#   key    = "${random_pet.this.id}-local"

#   file_source = "README.md"
#   #  content = file("README.md")
#   #  content_base64 = filebase64("README.md")

#   tags = {
#     Sensitive = "not-really"
#   }
# }