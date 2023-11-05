#!/bin/bash

if [ -n "${MINIO_USER}" ] && [ -n "${MINIO_PASSWORD}" ] && [ -n "${MINIO_ENDPOINT}" ]; then
    echo 'MC configuration set for "a"'
    export MC_HOST_a="http://${MINIO_USER}:${MINIO_PASSWORD}@${MINIO_ENDPOINT}"
fi
if [ -n "${SECOND_MINIO_USER}" ] && [ -n "${SECOND_MINIO_PASSWORD}" ] && [ -n "${SECOND_MINIO_ENDPOINT}" ]; then
    echo 'MC configuration set for "b"'
    export MC_HOST_b="http://${SECOND_MINIO_USER}:${SECOND_MINIO_PASSWORD}@${SECOND_MINIO_ENDPOINT}"
fi
if [ -n "${THIRD_MINIO_USER}" ] && [ -n "${THIRD_MINIO_PASSWORD}" ] && [ -n "${THIRD_MINIO_ENDPOINT}" ]; then
    echo 'MC configuration set for "c"'
    export MC_HOST_c="http://${THIRD_MINIO_USER}:${THIRD_MINIO_PASSWORD}@${THIRD_MINIO_ENDPOINT}"
fi
if [ -n "${FOURTH_MINIO_USER}" ] && [ -n "${FOURTH_MINIO_PASSWORD}" ] && [ -n "${FOURTH_MINIO_ENDPOINT}" ]; then
    echo 'MC configuration set for "d"'
    export MC_HOST_d="http://${FOURTH_MINIO_USER}:${FOURTH_MINIO_PASSWORD}@${FOURTH_MINIO_ENDPOINT}"
fi