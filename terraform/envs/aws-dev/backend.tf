terraform {
  backend "s3" {
    bucket         = "REPLACE_ME_beacon_tf_state"
    key            = "beacon/aws-dev/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "REPLACE_ME_beacon_tf_lock"
    encrypt        = true
  }
}
