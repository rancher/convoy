FROM ubuntu:15.04
MAINTAINER Sheng Yang <sheng.yang@rancher.com>

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
        python-pip \
        python-rtslib-fb \
        net-tools \
        iproute2 \
        curl \
        dnsutils \
        open-iscsi \
        mdadm
RUN pip install flask-restful

COPY shorthorn* /usr/local/bin/
RUN chmod a+x /usr/local/bin/shorthorn*
