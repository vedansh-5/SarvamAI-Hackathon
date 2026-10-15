#!/bin/sh
set -eu

profile=vans-deploy
region=ap-south-1
bucket=vans-releases

if aws s3api head-bucket --profile $profile --region $region --bucket $bucket 2>/dev/null; then
  echo "$bucket is already there."
else
  aws s3api create-bucket --profile $profile --region $region --bucket $bucket \
    --create-bucket-configuration LocationConstraint=$region >/dev/null
  echo "Made $bucket."
fi
aws s3api put-public-access-block --profile $profile --region $region --bucket $bucket \
  --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
aws ssm put-parameter --profile $profile --region $region --overwrite --type String \
  --name /vans/server/VANS_RELEASES_BUCKET --value $bucket >/dev/null
echo "The server reads releases from $bucket once it next starts; ./infra/aws/deploy-server.sh restarts it."
