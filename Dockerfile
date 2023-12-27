FROM alpine
RUN apk add --no-cache wireguard-tools
COPY wg2 /bin/wg2
ENTRYPOINT [ "/bin/wg2" ]