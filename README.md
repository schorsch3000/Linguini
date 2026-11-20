# Linguini

A quick translation popup built on [rofi](https://github.com/davatorium/rofi).
Press your hotkey, type a word or phrase, hit Enter, and copy the translation
or type it straight into the window you were working in.

```
de⇄en  Gift▏
Enter translate   Alt+t swap   Alt+s from   Alt+o to   Esc quit

de⇒en  Gift   via dictcc
  poison   noun · [chem.] [med.]
  venom    noun · [zool.]
```

- **Guesses the direction** between your home and foreign language. Unclear
  words like *die* or *Gift* are translated from your home language.
- **Dictionary results for single words**: several meanings with part of
  speech and back-translations. Phrases get a straight translation.
- **Many backends, tried in order**: free online services, official APIs,
  any OpenAI-compatible LLM, and fully offline options.
- **Fast**: a single static binary that starts in a few milliseconds.

## Requirements

- `rofi`
- `xclip` or `xsel`, for copying
- `xdotool`, for typing the result into a window (optional)

Copying and typing use X11 tools.

## Installation

### Release binary

Download the archive for your architecture from the
[releases page](https://github.com/schorsch3000/Linguini/releases), then:

```sh
tar xzf linguini-*-linux-amd64.tar.gz
install -Dm755 linguini-*/linguini ~/.local/bin/linguini
```

### With Go

```sh
go install github.com/schorsch3000/Linguini/cmd/linguini@latest
```

### From source

```sh
git clone https://github.com/schorsch3000/Linguini
cd Linguini
make install    # installs to ~/.local/bin
```

## Getting started

1. Bind `linguini` to a key in your window manager or desktop's keyboard
   shortcut settings. To translate the current selection instead, bind:

   ```sh
   sh -c 'linguini "$(xclip -o -selection primary)"'
   ```

2. Optionally create a config. Without one, Linguini translates between German
   and English using Google's free endpoint.

   ```sh
   mkdir -p ~/.config/linguini
   cp config.example.toml ~/.config/linguini/config.toml
   ```

   Set your languages at the top:

   ```toml
   home    = "de"
   foreign = "en"
   ```

## Usage

The prompt shows the translation direction: `de⇄en` while Linguini still
guesses it from your text, `de⇒en` once you have set it yourself.

**Input**

| Key          | Action                                        |
|--------------|-----------------------------------------------|
| Enter        | translate                                     |
| Alt+t        | swap the direction                            |
| Alt+s        | choose the source language                    |
| Alt+o        | choose the target language                    |
| Escape       | quit                                          |

Your text is kept when you swap or pick a language.

**Result**

| Key          | Action                                        |
|--------------|-----------------------------------------------|
| Enter        | copy the selected translation                 |
| Shift+Enter  | type it into the focused window               |
| Alt+t/s/o    | change the direction and translate again      |
| Alt+e        | back to the input to edit your text           |
| Escape       | quit                                          |

All keys can be changed in the `[keys]` section of the config.

## Backends

Linguini tries the backends listed in `[backend]` in order and shows the
first result. Phrases and single words have separate lists. Backends you have
not set up are skipped, so you only configure the ones you want.

| Backend        | Cost                                    | Notes                                   |
|----------------|-----------------------------------------|-----------------------------------------|
| `google`       | free, no account                        | unofficial endpoint, may rate-limit     |
| `azure`        | free tier: 2M characters/month          | dictionary results for words            |
| `google_cloud` | free tier: 500k characters/month        | needs Google Cloud billing enabled      |
| `deepl`        | DeepL API plan                          |                                         |
| `llm`          | depends on provider; free when local    | OpenAI, Ollama, llama.cpp, LM Studio, … |
| `bergamot`     | free, offline                           | phrases; few language pairs             |
| `dictcc`       | free for personal use, offline          | single words, best for German–English   |

API keys go in the backend's section as `api_key`, as a command that prints
it (`api_key_cmd = "pass show azure-translator"`), or in an environment
variable (`AZURE_TRANSLATOR_KEY`, `GOOGLE_CLOUD_API_KEY`, `DEEPL_API_KEY`,
`OPENAI_API_KEY`).

### Azure (recommended free fallback)

Create a *Translator* resource in the Azure portal with the free F0 tier and
copy its key and region:

```toml
[azure]
api_key_cmd = "pass show azure-translator"
region = "westeurope"
```

### LLM

Any OpenAI-compatible API works:

```toml
[llm]
base_url = "https://api.openai.com/v1"
model = "<model id>"
api_key_cmd = "pass show openai"
```

For a local model with Ollama:

```toml
[llm]
base_url = "http://localhost:11434/v1"
model = "llama3.2"
```

Add `"llm"` to the `[backend]` lists where you want it.

### Offline: dict.cc

Request the German–English dictionary download from [dict.cc](https://www.dict.cc)
(free for personal use) and save the unpacked text file as
`~/.local/share/linguini/dictcc-de-en.txt`, or set `[dictcc] file`.

The first lookup builds an index in `~/.cache/linguini`, which takes a few
seconds; after that, lookups are instant. With the dictionary installed,
Linguini also uses it to tell German and English words apart.

### Offline: Bergamot

Install [translateLocally](https://github.com/XapaJIaMnu/translateLocally/releases)
(a `.deb` is available) and download the models you need:

```sh
translateLocally -d de-en-base
translateLocally -d en-de-base
```

## Configuration

See [`config.example.toml`](config.example.toml) for every option with its
default value. Notable ones:

- `output.selections`: where Enter copies to: `clipboard`, `primary`,
  `secondary`, or several.
- `output.type_delay_ms`: delay between keystrokes when typing.
- `rofi.args`: extra rofi arguments, for example a theme:
  `["-theme", "~/.config/rofi/linguini.rasi"]`.
- `detect.margin`: how clearly text must look foreign before it is
  translated into your home language.

## Troubleshooting

- **rofi complains about a key binding:** one of Linguini's keys is already
  bound in your rofi config. Choose another key under `[keys]`, or add the
  rofi binding to `rofi.unbind`, e.g. `["kb-accept-alt", "kb-move-word-back"]`.
- **"HTTP 429" from google:** the free endpoint is rate-limiting you. Add
  another backend after it, e.g. `azure`.
- **Typed text ends up in the wrong window:** raise `output.type_wait_ms`.
- **Errors** are shown in a rofi window and printed to stderr; run `linguini`
  from a terminal to see them.

## Development

```sh
make test     # run the tests
make lint     # gofmt and go vet
make build    # build ./linguini
make dist     # release archives in dist/
```

Every push to `master` runs the tests and builds the archives. Pushing a tag
like `v1.2.0` publishes a GitHub release with the binaries.

## License

MIT, see [LICENSE](LICENSE).
