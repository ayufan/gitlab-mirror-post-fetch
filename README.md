gitlab-mirror-post-fetch
========================

Post fetch app compatible with https://github.com/dustin/gitmirror

## Deploy to Heroku

1. Create GitLab user (eg. GitMirror).
1. Create GitLab group (eg. Mirrors) and give GitLab user owner permissions.
1. Find GitLab user private token: https://my.gitlab.instance.com/profile/account
1. Create SSH private key and add it's public part to your GitLab
1. [![Deploy](https://www.herokucdn.com/deploy/button.png)](https://heroku.com/deploy)
1. And fill the forms:
	- **GITLAB_PRIVATE_TOKEN**: Private token
	- **GITLAB_URL**: Address of your GitLab instance
	- **GITLAB_GROUP**: Where all mirrors should be created
	- **GITLAB_SSH_KEY**: Paste here SSH private key
	- **GITMIRROR_SECRET**: Generate unique password that will secure your GitMirror installation
1. Click **Deploy For Free**
1. Open GitHub on project that you want to mirror. Go to *Settings* -> *Webhooks & Services* -> *Add Webhook*:
	- **Payload URL**: Paste URL of your application and add: **/callback/github**, ie.: *https://my-heroku-app.herokuapp.com/callback/github*
	- **Secret**: Enter your unique password, the same as GITMIRROR_SECRET.
	- Select **Just the push event**.
	- **Add Webhook**.
1. Within a couple of seconds mirror should be created and should be run in sync.
1. In case of error you can check *Recent Deliveries* and *Response*.

## Deploy to Tutum

1. Create GitLab user (eg. GitMirror).
1. Create GitLab group (eg. Mirrors) and give GitLab user owner permissions.
1. Find GitLab user private token: https://my.gitlab.instance.com/profile/account
1. Create SSH private key and add it's public part to your GitLab
1. [![Deploy to Tutum](https://s.tutum.co/deploy-to-tutum.png)](https://dashboard.tutum.co/stack/deploy/)
1. Click **Create and Deploy**
1. Go to service settings and edit environment variables:
	- **GITLAB_PRIVATE_TOKEN**: Private token
	- **GITLAB_URL**: Address of your GitLab instance
	- **GITLAB_GROUP**: Where all mirrors should be created
	- **GITLAB_SSH_KEY**: Paste here SSH private key
	- **GITMIRROR_SECRET**: Generate unique password that will secure your GitMirror installation
1. Open GitHub on project that you want to mirror. Go to *Settings* -> *Webhooks & Services* -> *Add Webhook*:
	- **Payload URL**: Paste URL of your application and add: **/callback/github**, ie.: *http://tutum-ip-address/callback/github*
	- **Secret**: Enter your unique password, the same as GITMIRROR_SECRET.
	- Select **Just the push event**.
	- **Add Webhook**.
1. Within a couple of seconds mirror should be created and should be run in sync.
1. In case of error you can check *Recent Deliveries* and *Response*.

## Advanced Installation

```go get github.com/ayufan/gitlab-mirror-post-fetch```

### The Use

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

## Author

Kamil Trzciński, [Polidea](http://www.polidea.com), 2014-2015

## License

MIT

