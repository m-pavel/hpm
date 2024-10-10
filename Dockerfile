FROM gcr.io/distroless/base
COPY ./hpm /hpm
ENTRYPOINT [ "/hpm" ]