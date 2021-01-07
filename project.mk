# Project specific values
OPERATOR_NAME?=$(shell sed -n 's/^projectName:\ \(.*\)/\1/p' PROJECT)
OPERATOR_NAMESPACE?=openshift-monitoring

IMAGE_REGISTRY?=quay.io
IMAGE_REPOSITORY?=$(USER)
IMAGE_NAME?=$(OPERATOR_NAME)

VERSION_MAJOR?=0
VERSION_MINOR?=1
