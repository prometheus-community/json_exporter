FROM golang:1.8.3
ADD . /app
WORKDIR /app
RUN ./gow get .
RUN ./gow build -o json_exporter . 
RUN chmod +x ./json_exporter && chmod +x ./entrypoint.sh

EXPOSE 7979
ENTRYPOINT ["./entrypoint.sh"]
