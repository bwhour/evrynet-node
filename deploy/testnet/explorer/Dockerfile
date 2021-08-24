FROM node:7

WORKDIR "/opt/explorer"
COPY ./web/ ./
COPY ./env_setup.sh .
RUN chmod +x env_setup.sh

RUN npm install
CMD ["/bin/bash", "./env_setup.sh"]

EXPOSE 8080