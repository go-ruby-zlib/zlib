# Examples

Runnable pure-Ruby usage of the `zlib` library, verified under the [rbgo](https://github.com/go-embedded-ruby) interpreter.

```sh
rbgo examples/zlib_usage.rb
```

| File | Shows |
| --- | --- |
| `zlib_usage.rb` | One-shot `Zlib.deflate`/`inflate` and `Zlib.gzip`/`gunzip` round-trips, `Zlib.crc32`/`adler32` checksums, and the streaming `Zlib::Deflate`/`Zlib::Inflate` API with `total_out`. |
