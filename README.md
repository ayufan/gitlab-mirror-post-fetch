gitlab-mirror-post-fetch
========================

Post fetch app compatible with https://github.com/dustin/gitmirror

## Installation

```go get github.com/ayufan/gitlab-mirror-post-fetch```

## The Use

1. Create GitLab user (eg. GitMirror).
1. Create GitLab group (eg. Mirrors) and give GitLab user owner permissions.
1. Find GitLab user private token: https://my.gitlab.instance.com/profile/account
1. Install https://github.com/dustin/gitmirror
1. Create `bin/post-fetch` script filling in the blanks:

```
#!/bin/bash

export GITLAB_PRIVATE_TOKEN=MY_GITMIRROR_USER_PRIVATE_TOKEN
export GITLAB_URL=https://my.gitlab.instance.com/
export GITLAB_GROUP=Mirrors

exec gitlab-mirror-post-fetch
```

1. Give `bin/post-fetch` executable permissions: `chmod +x bin/post-fetch`
1. Configure `gitmirror` script as described. Giving it or not `secret`.

## Deploy to Heroku

[![Deploy](https://www.herokucdn.com/deploy/button.png)](https://heroku.com/deploy)

## Author

Kamil Trzciński, Polidea, 2014

## License

MIT

