# simp
```bash
echo 'Tell a joke.' | simp
```

Simp is a simple point of consumption and an ergonomic CLI tool for many OpenAI-compatible, but also _the incompatible_ inference backends; it's somehow similar to [llm][0] by Simon Willison but definitely _not_ inspired by it. (Funny: I did the first version of this program the day before Simon did his.) Consider [vim-simp][1] to bring hot agents to your scratch buffers.

If you're anything like me, you will love it!

There's also `simp -daemon` which is, like, a whole API gateway thing; it supports OpenAI, Anthropic, Gemini, Vertex, and Dify drivers for now. OpenAI driver itself covers a surprising number of backends, including most self-hosted ones. [Jina][16] comes to mind; note that `task` and `late_chunking` parameters are supported natively due to `simp` using [a fork][17] of [go-openai][18].

Simp uses the configured keychain by default, however in the future I'll be adding compartmentation, and SSO via Cloudflare, JWT, OIDC, you name it; it also follows OpenAI's [Batch API][2] in provider-agnostic manner, & default to normal endpoints if some provider won't support it.

The tool is designed with ergonomics in mind.

```bash
git clone --recursive https://github.com/busthorne/simp
cd simp
go install ./cmd/simp
simp -configure
echo 'Tell a joke.' | simp
# gpt-4o [temperature, [max_length, [top_p, [frequency_penalty, [presence_penalty]]]]]
echo 'Tell a joke.' | simp 4o 0.5 200 1 0 0
```

## Features
- [x] [Cables](#cable-format)
	- [ ] Polycables
- [x] Drivers
	- [x] OpenAI
	- [x] Anthropic
	- [x] Gemini
	- [x] Vertex
	- [ ] Bedrock
	- [ ] [Dify][10]
		- [ ] [Conversations](#dify)
- [x] Interactive mode
- [x] [Daemon mode](#daemon)
	- [x] OpenAI-compatible API
	- [x] [Universal Batch API](#batch-api)
		- [x] OpenAI
		- [x] Anthropic
		- [x] [Vertex](#vertex)
	- [ ] SSO
- [x] [Vim mode][1]
- [x] Wizard
- [x] Keyring
	- [x] KWallet
	- [x] GNOME Keyring
	- [x] Pass
	- [x] macOS Keychain
	- [x] wincred
	- [x] libsecret
	- [x] keyctl
	- [x] file
- [x] [History](#history)
	- [x] Grouping
	- [x] Ignore
	- [x] [Annotations](#annotations)
- [ ] Tools
	- [ ] [RAG](#rag)
	- [ ] CLI
	- [ ] Universal function-calling semantics
	- [ ] Diff mode
	- [ ] Vim integration

## Cable format
I'd designed a special-purpose plaintext _cable_ format to support interactive chats and histories.

```cable
You are helpful assistant.

	>>>
Tell a joke.

	<<< cs35
Chicken crossed the road.
```

Even though `simp` accepts non-cable input, it will always try to parse the cable first. The parser is clever to avoid ambiguity: if it deems the input a malformed cable, it will err explicitly. Keep in mind that non-cable input is treated as a single user message; optionally, the system message is populated from `-system` argument. The cable format uses tabular `>>>` and `<<<` guidelines to indicate user and assistant messages respectively. If there's text preceding the first `>>>` prompt, it's considered a system message.

The guidelines would sometimes contain metadata. For example, `<<< alias` indicates what model (or the shortest configured alias thereof) was used to produce the message, and conversely `>>> alias` is how you would _explictly_ switch the model mid-conversation. The metadata is not mandatory, however it also helps a lot to keep track of who's who; in interactive mode, `simp` will use the last explictly prompted model for subsequent completions.

If a given user message contains png, jpeg, gif, or webp links, `simp` would will rewrite to structure it as multipart.

The idea is that the format is simple enough for it to be used in a notepad application, and shell alike; because it's not a structured data format, shell scripts could multi-turn by appending chat completions to prompts, or maintain some buffer, if necessary. This also means that the cable format could be easily integrated with the editor of your choice. Take a look at [vim-simp][1] which is a _really_ simple program all things considered.

### Polycables
If you wish to compare multiple models at the same time, or maintain multiple alternate versions of the same chat, you should try _polycables_. The plaintext medium restricts the space of possible solutions for this problem. As far as readability is concerned, and indeed for `simp` it is the case, guidelines must remain visually aligned. I'm sure some will like it, and some would hate it, but it's my design so I make the call: the guidelines are modified to indicate the alternate number, so a continuation `<<<` becomes `<1<` for the first, `<2<` for the second alternate, and so on.

In the chat below, the default model is used in the beginning as the model was not explicitly prompted. The user would then request three alternates, and decide to continue with the third alternate, albeit with a different model. If they used `>>>` like you would normally do, all three alternates would have continued further using the respective models. However, had they also prompted `>>> model` then all three alternates would have continued using `model`.

```cable
You are helpful assistant.

	>>>
Write a one-sentence setup for a joke.

	<<< 4o
A soldier, scientist, and politican walk into a bar.

	>>> cs35,4o,flash
Write a punchline.

	<1< cs35
The bartender says "What is this, some kind of international crisis?"

	<2< 4o
The bartender looks up and says, "Is this a strategy meeting or just another round of 'whose job is harder'?"

	<3< flash
...and the bartender says, "What is this, some kind of joke?"

	>3> pro
Write this joke in Ukrainian verbatim.

	<3<
Солдат, вчений та політик заходять у бар... і бармен каже: "Що це, якийсь жарт?"
```

In Vim mode, `simp` will always terminate output with a trailing prompt marker.

Polycables gather in parallel, so the first alternate is streamed until completion, then whatever was generated in the second alternate up to that point is echoed at once, streaming the rest until completion, and so on. Interactive mode does not allow re-writing past messages due to limitations of the terminal, however in [vim-simp][1] it's possible, and indeed really valuable.

Revisiting past cables later, and partially rewriting it with new context is very powerful; see [History](#history) to learn how you could use `simp` to preserve, and catalogue past cables, as well as using [ripgrep][3] and [fzf][4] to navigate them efficiently. In future releases `simp -daemon` will support some form of [RAG](#rag) books, courtesy of [sqlite-vec][6] extensions, in terms of [tool-use](#tools). The tool support will be cables-first. The functions and [GBNF][19] grammars could be read from the system message; see my [PR][20] in Ollama for reference, while the scope of RAG (files, collections of files, i.e. _codebase_ search) parsed from guideline metadata.

## Daemon
```bash
simp -daemon
```

The CLI tool will function with or without a daemon.

I'd written a hanful of OpenAI-compatible API proxies over the years, given that it's constantly changing: contrary to what it may seem, it's not the easiest task in the world. There's plenty of proxies on Github, however I'd been really surprised at some design choices they're making, and really couldn't dig any of them.

In particular, I'd always wanted a proxy that could do [Batch API][2], too.

Currently, simp in both CLI and daemon mode only tracks the models that have been explictly defined, however this is about to change soon with all the state-keeping changes; some providers have list endpoints, others don't but it's really killing latency to poll for model list to route providers... This is why some kind of per-provider list cache is needed, and while I'm at it, I thought I might do state properly. Well, at least it reload during run-time on config changes so you may introduce new changes without having to restart the whole thing. However, this may not necessarily be the case if involving some keychains.

### RAG
I've been quite impressed with [Cursor][7] recently, and in particular it's diff and codebase search features. This is something that is missing from my Vim workflow, and if I were to replicate it, I would be very happy. This was the primary motivator, in fact, behind the daemon. (The other is having a provider-agnostic batching proxy.) Now again, there's many RAG providers, vector databases, and whatnot. Even though I may support _some_ of them in the future (but probably just [pgvector][8] and [pgvecto.rs][9] which is what I use) through a generic configuration interface, Simp is currently using [sqlite-vec][6] so that's where RAG will go when it eventually lands master.

I'm not sure I like the "index all" that is not in `.cursorignore` approach, though.

The final implementation will be determined by the strength and limitations of Vim, but I think it would be straightforward to port it to Visual Studio Code, too, and somebody had already suggested they might do it, so I'll keep that in mind. With RAG enabled, I would be adding more code-aware metadata to the cable format: so that diff's could be resolved transparently, and buffers aggregated for context. Good news is this year I'd learnt how to do chunk code up, progressively summarise it, and arrange it in something more reasonable than a vector heap, and that also requires a proper database. There's not necessarily Postgres to help me here, but I'm quite confident I will get there with SQLite.

### Dify
[Dify][10] is a source-available SaaS based on [React Flow][21] which is similar to Langflow, Flowise, but IMHO much better.

I'm amazed how despite shitty code, it proves increasingly versatile time and time again. Analysts at [Busthorne][22] use it quite extensively as it doesn't really require programming background. For example, we have multiple SQL knowledge bases and agents that are able to write, execute, & analyse the results of SQL queries. Most important of all, it uses Postgres for everything, so I'm also able to query the KB's, and use [pgvecto.rs][9] indexing, too. We have built out extensive language-games infrastructure based on Dify.

There's value in proxying Dify agents like `simp` does with the inference drivers, however the API doesn't allow arbitrary conversations. The chatflows in Dify have hidden state that they're able to modify, so the conversations cannot be created out of the blue. Thankfully, `simp` has SQLite book-keeping now, so we might figure out something out; editing the conversations table in Postgres directly under Dify's nose is also on the table.

The driver implementation is there but it's not exactly ready for consumption.

### Configuration
Simp tools will use `$SIMPPATH` that is set to `$HOME/.simp` by default.

For basic use, interactive `simp -configure` is sufficient.

The config files are located in `$SIMPPATH/config` in [HCL][5] format, so Terraform users will find it familiar! The state is contained in `$SIMPPATH/books.db3` which is a SQLite database with vector extension. The config is structured to allow various keychain, authentication, and authorization backends: each provider gets its own view of the keychain, allowing it to persist various settings so as not to impact others.

```hcl
default {
	model = "cs35"
	temperature = 0.7
}

daemon {
	listen_addr = "0.0.0.0:51015"
	allowed_ips = ["10.0.0.0/8"]

	# or
	daemon_addr = "http://server-on-the-network.lan:51015"
}

auth "keyring" "default" {
	backend = "keychain" # use macOS system keychain
}

provider "openai" "api" {
	model "gpt-4o" {
		alias = ["4o"]
	}
	model "o1-preview" {
		alias = ["o1"]
	}
}

provider "vertex" "api" {
	project = "simp"
	region  = "europe-north1"
	bucket  = "simpmedia"
	dataset = "simpbatches"
	batch   = true

	model "gemini-1.5-flash-002" {
		alias = ["flash"]
	}
	model "gemini-1.5-pro-002" {
		alias = ["pro"]
	}
}

provider "anthropic" "api" {
	model "claude-3-5-sonnet" {
		alias = ["cs35"]
		latest = true # simp will append -latest on the wire
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
	}
}

history {
	annotate_with = "ch35"

	# group histories from all projects by project name
	path "~/projects/*/**" {
		group = "*"
	}
	# except this
	path "~/projects/simp" {
		ignore = true
	}
}
```

### Batch API
OpenAI has introduced [Batch API][2]—a means to perform many completions and embeddings at a time at 50% discount. Anthropic and Google have since followed. However, neither provider's API matches the other. This presents a challenge: if Google were to release a ground-breaking model, my OpenAI-centric code would be worthless. Because the daemon is a provider-agnostic, API gateway, it's well-positioned to support batching in provider-agnostic and model-agnostic fashion!

Normally, a provider would require any given batch to be completions-only, or embeddings-only, or one model at a time (Google is like that!)

Since `simp -daemon` has to translate batch formats regardless, it allows batches with arbitrary content, and arbitrary providers. You may address OpenAI, Anthropic, _and_ Google models all in the same batch, but also even the providers that _do not_ support batching. In that case, the dameon would treat your batch as "virtual", partition it into per-model chunks, and gather them on completion. If the model provider referenced in the batch does not support batching natively, the relevant tasks will trickle down at quota speed using normal endpoints. To consumer this behaviour is transparent; you get to create one OpenAI-style batch using whatever configured models (aliases) or providers, & the daemon would take care of the rest.

> If you work with text datasets as much as I do, my money is you would find this behaviour as _liberating_ at least as much as I do. Although you should note that the implementation is quite complex, so there may be bugs. I have done end-to-end testing, and dogfood eat everyday, but I cannot guarantee that your big batch won't go bust!

#### Vertex
[Batch API][15] in Google's Vertex AI requires a BigQuery dataset for communicating back the results. Well, there's the bucket option, however it's really messy to match `custom_id` from the responses, as they're out-of-order, and that would make it really complicated to handle. Therefore, by default Batch API is disabled for Vertex providers, see [Configuration](#configuration) for example configuration that covers all.

The following Gemini models support multimodal [fileData][19] inputs:
* gemini-1.5-flash-002
* gemini-1.5-flash-001
* gemini-1.5-pro-002
* gemini-1.5-pro-001

OpenAI API only offers `text` and `image_url` parts for the time being, so that's what Simp is using. However, the actual URL under `image_url` could be a text/plain, application/pdf, image, audio, or video files.

```json
{
	"contents": [{
			"role": "user",
			"parts": [
				{"text": "What is the relation between the following video and image samples?"},
				{"fileData": {
					"fileUri": "gs://cloud-samples-data/generative-ai/video/animals.mp4",
					"mimeType": "video/mp4"
				}},
				{"fileData": {
					"fileUri": "gs://cloud-samples-data/generative-ai/image/cricket.jpeg",
					"mimeType": "image/jpeg"
				}}
			]
	}]
}
```

Vertex mandates that `fileUri` is a Cloud Storage path, so unless the URL starts with `gs://` Simp will download the file, infer its `mimeType` from either Content-Type, or the file extension, and write it down to the configured bucket under SHA256 content-addressible filename.

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

## Tools
Tool-use is most exciting: I really hate how the major providers are doing it, & unfortunately there's limitations to what you can accomplish without control of the inference-time. Therefore, `simp` will likely only support model-in-the-loop tools with grammar sampling and K/V backtracking capabilities.

**Key idea** is whereas you did round-trip endpoints per-call, you would have one, long model-in-the-loop inference; see [AICI][12] by Microsoft for inspiration on how this could be done. So whenever the model makes a call, you parse it mid-completion, pause, go back to a K/V checkpoint before the call, and re-write with the result, carry on. The traditional API like OpenAI's—would lead you to believe that for a single chat completion with N sequential function-calls, N+1 round-trip inferences are necessary.

However, this is only the case because your chat context has to go through the API line multiple times. If you retained control of the context window during inference, you could match its output grammatically in realtime! This is even more powerful when considering the parallel function-calling capability. The backtracking will reduce, or hide latency from network calls completely. Moreover, everything here lends to batching nicely: compute reasoning steps for parallel-calls from the same position in K/V cache. Also known as predictive branching. Have a reward model assign scores to reasoning, too; congratulations, your sampler implements [MCTS][14] with an actual reward function.

I don't have a clear implementation strategy for tool-use in simp, but it would have to tick the following boxes:

- [ ] Tools are well-defined [HCL][5]
	- [ ] OpenAPI specs from JSON/YAML definitions
	- [ ] Agents as tools
	- [ ] Interruptible work
	- [ ] [Grammars][19]
	- [ ] WebAssembly-like controller API, see [AICI][12]?
- [ ] CLI semantics
	- [ ] Prerequisite: diff mode for fixing calls, editing code, etc.
	- [ ] Shell command-tools require `[y/n]` consent
	- [ ] Vim mode support
- [ ] API semantics

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
[11]: https://github.com/busthorne/pg_bluerose
[12]: https://github.com/microsoft/aici
[13]: https://en.wikipedia.org/wiki/Directed_acyclic_graph
[14]: https://en.wikipedia.org/wiki/Monte_Carlo_tree_search
[15]: https://cloud.google.com/vertex-ai/generative-ai/docs/multimodal/batch-prediction-gemini
[16]: https://jina.ai/news/jina-embeddings-v3-a-frontier-multilingual-embedding-model/#parameter-latechunking
[17]: https://github.com/busthorne/go-openai
[18]: https://github.com/sashabaranov/go-openai
[19]: https://github.com/ggerganov/llama.cpp/blob/master/grammars/README.md
[20]: https://github.com/ollama/ollama/pull/7513
[21]: https://reactflow.dev/
[22]: https://github.com/busthorne
