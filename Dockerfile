FROM node:11-alpine
WORKDIR /usr/src/app
COPY app/package.json .
COPY app/yarn.lock .
RUN yarn install --only=production
COPY app .
CMD ["yarn", "start"]