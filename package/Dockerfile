FROM ubuntu:15.10
MAINTAINER Sheng Yang <sheng.yang@rancher.com>

RUN apt-get install -y \
        libaio1

ENV CONVOY_VERSION v0.3.1-rc1
ADD https://github.com/rancher/convoy/releases/download/${CONVOY_VERSION}/convoy.tar.gz /tmp/
RUN tar xvf /tmp/convoy.tar.gz -C /tmp/ \
  && cp /tmp/convoy/convoy* /usr/local/bin/ \
  && rm /tmp/convoy.tar.gz

ADD convoy-start /usr/local/bin/
RUN chmod a+x /usr/local/bin/convoy-start
