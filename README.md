# simp
```bash
echo 'Tell a joke.' | simp
```

Simp is a simple point of consumption and an ergonomic CLI tool for many OpenAI-compatible, but also _the incompatible_ inference backends; it's somehow similar to [llm][0] by Simon Willison but definitely _not_ inspired by it. (Funny: I did the first version of this program the day before Simon did his.)

Consider [vim-simp][1] to bring hot agents to your scratch buffers. If you're anything like me, you will love it!

There's also `simp -daemon` which is, like, a whole API gateway thing; it only supports Anthropic and OpenAI drivers for now (which supports a surprising number of backends, including most self-hosted ones.) It uses the configured keychain by default, however in the future I'll be adding compartmentation, and SSO (via Cloudflare, JWT, OIDC, you name it.) But perhaps most notably, as soon as I'm able to figure out the most elegant way to handle state ([sqlite-vec][6]?) it will follow OpenAI's [Batch API][2] in provider-agnostic manner, & default to normal endpoints if some provider won't support it.

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
- [x] Drivers
	- [x] OpenAI
	- [x] Anthropic
	- [x] Gemini
	- [x] [Dify][10]
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
	- [x] Group by path parts
	- [x] Ignore
	- [x] [Annotations](#annotations)
- [ ] Diff mode
	- [ ] Vim integration
- [ ] [RAG](#rag)
- [ ] Funny: CI builds, cross-compilations, and everything

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

The CLI tool will function with or without a daemon.

I'd written a hanful of OpenAI-compatible API proxies over the years, given that it's constantly changing: contrary to what it may seem, it's not the easiest task in the world. There's plenty of proxies on Github, however I'd been really surprised at some design choices they're making, and really couldn't dig any of them.

In particular, I'd always wanted a proxy that could do [Batch API][2], too.

Currently, simp in both CLI and daemon mode only tracks the models that have been explictly defined, however this is about to change soon with all the state-keeping changes; some providers have list endpoints, others don't but it's really killing latency to poll for model list to route providers... This is why some kind of per-provider list cache is needed, and while I'm at it, I thought I might do state properly.

### RAG
I've been quite impressed with [Cursor][7] recently, and in particular it's diff and codebase search features. This is something that is missing from my Vim workflow, and if I were to replicate it, I would be very happy. This was the primary motivator, in fact, behind the daemon. (The other is having a provider-agnostic batching proxy.)

Now again, there's many RAG providers, vector databases, and whatnot. Even though I might only support _some_ of them (but probably just [pgvector][8] and [pgvecto.rs][9] which is what I use) through a generic configuration interface, the build will most definitely include [sqlite-vec][6] because state.

I'm not sure I like the "index all" that is not in `.cursorignore` approach, though.

The final implementation will be determined by the strength and limitations of Vim, but I think it would be straightforward to port it to Visual Studio Code, too, and somebody had already suggested they might do it, so I'll keep that in mind. With RAG enabled, I would be adding more code-aware metadata to the cable format: so that diff's could be resolved transparently, and buffers aggregated for context. Good news is this year I'd learnt how to do chunk code up, progressively summarise it, and arrange it in something more reasonable than a vector heap, and that also requires a proper database.

There's not necessarily Postgres to help me here, but I'm quite confident I will get there with SQLite.

### Configuration
Simp tools will use `$SIMPPATH` that is set to `$HOME/.simp` by default.

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
provider "openai" "ollama" {
	base_url = "http://127.0.0.1:11434/v1"
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
	annotate_with = "ch35" # see. claude-3-5-haiku
}
```

## History
This is perhaps the biggest upgrade since the olden days of `simp` when—trivia—it was still called `gpt`.

The history option is basically making sure your cables are not evaporating when you want them to be remembered, cherished, and otherwise organised—this is why we have `$SIMPPATH/history` folder. By default, all simp cables are stored there. You can control how they're grouped in the config: by specifying paths and grouping expressions, which will be used to catalogue the cables appropriately. Unfortunately, it's all relative to history directory now. Absolute paths will be added at some point. 

Note that you may use wildcards.

For example, you could group the cables produced in `/opt/projects` by top-level project directory name. In that case, you would use `/opt/projects/*/**`, and `*` in the group expression. If you had a project called "simp", all cables created in `/opt/projects/simp` would be saved under `simp/` in the history directory. Without the `**` in the path expression, the the children would not be considered for grouping, however this is not the case for ignore: paths are ignored inclusively.

The longest-prefix match wins; you may similarly configure to ignore certain paths for good.

```bash
cd my/project/path
simp -historypath # will tell you where your cables are going
```

Who knows, we might add regular expressions someday :-)

### Annotations
This is exactly the feature that triggered me to obsessively re-write the tool, and bring it to its current state.

Basically, if you'll be organising your chat histories, you might as well chip in with Haiku or something local like that, to write filenames for your cables. It's genius, I know, but it's also true. That is what the `annotate_with` option for. Again, you could imagine how this would play into rudimentary RAG, sharing knowledge between scratch buffers, between projects, and so on. I'm quite optimistic that this "idea" would go a long way.

I guess you might see now why I'd gone with [HCL][5].

## License
MIT

[0]: https://github.com/simonw/llm
[1]: https://github.com/busthorne/vim-simp
[2]: https://platform.openai.com/docs/guides/batch
[3]: https://github.com/BurntSushi/ripgrep
[4]: https://github.com/junegunn/fzf
[5]: https://github.com/hashicorp/hcl
[6]: https://alexgarcia.xyz/sqlite-vec/go.html
[7]: https://www.cursor.com/
[8]: https://github.com/pgvector/pgvector
[9]: https://github.com/tensorchord/pgvecto.rs
[10]: https://github.com/langgenius/dify
