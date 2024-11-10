# simp
```bash
echo 'Tell a joke.' | simp
```

Simp—a **sim**ulating **p**roxy—is a simple tool that provides a single point of consumption for many OpenAI-compatible, but also the incompatible inference backends; it's somehow similar to [llm][0] by Simon Willison but definitely not inspired by it. (Funny: I did the first version of this program the day before Simon did his.) Consider [vim-simp][1] to bring hot agents to your scratch buffers. If you're anything like me, you will love it! There's also `simp -daemon` which is, like, a whole API gateway thing, most notably supporting [Batch API][2] that is to say it keeps state, & absolutely will default to normal endpoints if some provider won't support it.

The tool is designed ergonomically so that you could bring it to shell scripts.

```bash
go install github.com/busthorne/simp/cmd/simp@latest
simp -configure
echo 'Tell a joke.' | simp
# gpt-4o [temperature, [max_length, [top_p, [frequency_penalty, [presence_penalty]]]]]
echo 'Tell a joke.' | simp 4o 0.5 200 1 0 0
```

## Features
- [x] Prompt from stdin, complete to stdout
- [x] Keychains
- [x] [Cables](#cable-format): multi-player, model-independent plaintext chat format
- [x] [Daemon mode](#daemon)
	- [x] OpenAI-compatible API gateway
	- [ ] Universal [Batch API][2]
	- [ ] SSO
- [x] Interactive mode
- [x] [Vim mode][1]
- [x] Setup wizard `-configure`
- [x] [History](#history)
	- [x] Group by path
	- [x] Annotations

## Cable format
Simp includes a special-purpose plaintext cable format for chat conversations.

The CLI tool accepts user input via stdin, and it will try to parse it using the cable format. The parser is clever: if the input would appear as cable format, however not well-formed, it will err. Otherwise, it will simply treat the entirety of input as the user message. The image URL's and Markdown image items are treated as images by default.

In Vim mode, `simp` will always terminate output with a trailing prompt marker.

```
You are helpful assistant.

	>>>
Tell a joke

	<<< 4o
Chicken crossed the road.

	>>>
You can switch models mid-conversation!

	<<< cs35
Chicken crossed the road a second time.
```

The idea is that the format is simple enough for it to be used in a notepad application, or shell; because it's not a structured data format, chat completions may be read from stdin and immediately appended to prompt, or concatenated with a buffer of some kind. This also means that the cable format is easy to integrate. Take a look at [vim-simp][1] it's a really simple program all things considered.

All your conversations are just plaintext files that you can [ripgrep][3] and [fzf][4] over.

## Daemon
```bash
simp -daemon
```



### Configuration
Simp tools will use `$SIMPPATH` that is set to `$HOME/.simp` by default. The CLI tool will function with or without a daemon, however a daemon is beneficial if you care about async features that `simpd` has to offer besides the API gateway; it uses [sqlite-vec][6] for persistence, and that only allows one writer at a time.

For basic use, interactive `simp -configure` is sufficient.

The config files are located in `$SIMPPATH/config` in [HCL][5] format, so Terraform users will find it familiar!

```hcl
default {
	model = "4o"
	temperature = 0.7
	# ... the usual suspects ...
}

daemon {
	listen_addr = "127.0.0.1:51015"
	#allowed_ips = ["10.0.0.0/8"]
}

# use macOS system keychain
auth "keyring" "default" {
	backend = "keychain"
	keychain_icloud = true
}

provider "openai" "api" {
	model "gpt-4o" {
		alias = ["4o"]
	}
	model "o1-preview" {
		alias = ["o1"]
		temperature = 1.0
	}
}

provider "anthropic" "api" {
	model "claude-3-5-sonnet" {
		alias = ["cs35"]
		latest = true
	}
	model "claude-3-5-haiku" {
		alias = ["ch35"]
		latest = true
	}
}
provider "openai" "llama-cpp-python" {
	base_url = "http://127.0.0.1:8000/v1"
	model "gemma-2-9b-simpo" {
		alias = ["g9s"]
		tags = ["q4_0", "q8_0"] # the first tag is the default
		context_length = 8192
		max_tokens = 4096
	}
	model "gemma-2-27b-it" {
		alias = ["g27"]
		tags = ["q4_0"]
		context_length = 8192
		max_tokens = 4096
	}
}

history {
	# group histories from all projects by project name
	path "~/projects/*/**" {
		group = "*"
	}
	# except this
	path "~/projects/simp" {
		ignore = true
	}
	annotate_with = "cs35" # see. claude-3-5-sonnet
}
```

## License
MIT

[0]: https://github.com/simonw/llm
[1]: https://github.com/busthorne/vim-simp
[2]: https://platform.openai.com/docs/guides/batch
[3]: https://github.com/BurntSushi/ripgrep
[4]: https://github.com/junegunn/fzf
[5]: https://github.com/hashicorp/hcl
[6]: https://alexgarcia.xyz/sqlite-vec/go.html