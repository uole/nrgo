FROM golang:1.20-alpine3.16 AS binarybuilder
USER root

ENV DIR=/go/nrgo

RUN go env -w GO111MODULE=on; \
    go env -w GOPROXY=https://goproxy.cn,direct; \
    sed -i 's/dl-cdn.alpinelinux.org/mirrors.ustc.edu.cn/g' /etc/apk/repositories; \
    mkdir -p ${DIR}; \
    apk add --update --no-cache git make

COPY . ${DIR}

WORKDIR ${DIR}

RUN make build


FROM alpine:3.16
USER root

ENV DIR=/go/nrgo

RUN apk add --update --no-cache tzdata

COPY --from=binarybuilder ${DIR}/bin/ /usr/bin/

EXPOSE 80

CMD ["nrgo"]