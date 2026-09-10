FROM quay.io/openshifttest/python:3.9

ARG SVT_REF=master
ARG OC_VERSION=4.12.60

RUN curl -sSL "https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${OC_VERSION}/openshift-client-linux.tar.gz" \
      | tar -xzf - \
    && mv oc /bin \
    && mv kubectl /bin \
    && apt-get update \
    && apt-get install -y gettext-base uuid-runtime jq \
    && ln -sf /bin/bash /usr/bin/bash \
    && /usr/local/bin/python -m pip install --upgrade pip \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /reliability-v2

RUN curl -sSL "https://github.com/openshift/svt/archive/${SVT_REF}.tar.gz" \
      | tar -xzf - --strip-components=2 "svt-${SVT_REF}/reliability-v2" \
    && pip install -r requirements.txt \
    && chmod +x start.sh \
    && chgrp -R 0 /reliability-v2 \
    && chmod -R g+rwX /reliability-v2
