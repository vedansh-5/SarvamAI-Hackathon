#!/bin/sh
set -eu

profile=vans-deploy
region=ap-south-1
bucket=${VANS_RELEASES_BUCKET:-vans-releases}

version=$(aws s3 cp --profile $profile --region $region "s3://$bucket/latest.json" - | python3 -I -c 'import json,sys;print(json.load(sys.stdin)["version"])')
echo "VANS $version, a link that works for 7 days:"
aws s3 presign --profile $profile --region $region "s3://$bucket/$version/VANS_$version.dmg" --expires-in 604800
