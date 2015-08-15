FROM ubuntu:14.04.2
MAINTAINER Sheng Yang <sheng.yang@rancher.com>

RUN apt-get install libaio1

ENV PDATA_TOOLS_VERSION rancher-v0.2
ADD https://github.com/rancher/thin-provisioning-tools/releases/download/${PDATA_TOOLS_VERSION}/pdata_tools \
        /usr/local/bin/pdata_tools
RUN chmod +x /usr/local/bin/pdata_tools

ENV RANCHER_VOLUME_VERSION v0.2-rc2
ADD https://github.com/rancher/rancher-volume/releases/download/${RANCHER_VOLUME_VERSION}/rancher-volume \
	/usr/local/bin/rancher-volume
RUN chmod +x /usr/local/bin/rancher-volume
ADD https://github.com/rancher/rancher-volume/releases/download/${RANCHER_VOLUME_VERSION}/rancher-mount \
	/usr/local/bin/rancher-mount
RUN chmod +x /usr/local/bin/rancher-mount

COPY rancher-volume-start /usr/local/bin/rancher-volume-start
