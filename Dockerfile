FROM golang:1.4
VOLUME /repos
RUN go get github.com/ayufan/gitlab-mirror-post-fetch
RUN go get github.com/ayufan/gitmirror
ADD /repos/bin/post-fetch /post-fetch
ENV GITLAB_PRIVATE_TOKEN "<enter-GitLab-user-private-token>"
ENV GITLAB_URL "https://gitlab.org/"
ENV GITLAB_SSH_KEY "Paste here ssh key assigned to GitLab user beggining with -----BEGIN RSA PRIVATE KEY-----"
ENV GITMIRROR_SECRET "<enter-random-secret-to-be-used-to-secure-gitmirror>"
CMD ["bash", "-c", "exec gitmirror -addr=:80 \"-secret=$GITMIRROR_SECRET\" -dir=/repos -post-fetch=/post-fetch"]
