#!/bin/bash
#
# As multistage build is not available in docker-ce 17.03 (supported by kubeadm 1.10),
# first build the plugin into a first container; then create the devplugin image 

echo Building accelerator-device-plugin:build

docker build --rm -t accelerator-device-plugin:build . -f Dockerfile.build

docker container create --name pluginbuild accelerator-device-plugin:build  
docker container cp pluginbuild:/go/bin/k8s-accelerator-devplugin .  
docker container rm -f pluginbuild

echo Building accelerator-device-plugin:latest

docker build --rm --no-cache -t accelerator-device-plugin:latest .
rm ./k8s-accelerator-devplugin
