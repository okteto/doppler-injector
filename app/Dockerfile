FROM node:11-alpine
WORKDIR /usr/src/app
COPY package.json .
RUN yarn install --only=production
COPY . .
CMD ["yarn", "start"]