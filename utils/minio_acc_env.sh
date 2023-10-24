#!/bin/bash

export TF_ACC=0
export MINIO_ENDPOINT=`docker network inspect bridge | jq -r .[].IPAM.Config[].Gateway`:9000
export MINIO_USER=minio
export MINIO_PASSWORD=minio123
export MINIO_ENABLE_HTTPS=false
export SECOND_MINIO_ENDPOINT=`docker network inspect bridge | jq -r .[].IPAM.Config[].Gateway`:9002
export SECOND_MINIO_USER=minio
export SECOND_MINIO_PASSWORD=minio321
export SECOND_MINIO_ENABLE_HTTPS=false
export THIRD_MINIO_ENDPOINT=`docker network inspect bridge | jq -r .[].IPAM.Config[].Gateway`:9004
export THIRD_MINIO_USER=minio
export THIRD_MINIO_PASSWORD=minio456
export THIRD_MINIO_ENABLE_HTTPS=false
export FOURTH_MINIO_ENDPOINT=`docker network inspect bridge | jq -r .[].IPAM.Config[].Gateway`:9006
export FOURTH_MINIO_USER=minio
export FOURTH_MINIO_PASSWORD=minio654
export FOURTH_MINIO_ENABLE_HTTPS=false