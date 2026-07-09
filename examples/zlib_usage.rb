# frozen_string_literal: true

require "zlib"

# A repetitive payload compresses well, which makes the ratio visible.
data = "The quick brown fox jumps over the lazy dog. " * 40

# One-shot raw DEFLATE round-trip.
compressed = Zlib.deflate(data, Zlib::BEST_COMPRESSION)
puts "deflate:  #{data.bytesize} -> #{compressed.bytesize} bytes"
puts "inflate:  restored? #{Zlib.inflate(compressed) == data}"

# One-shot gzip round-trip (adds the gzip header/footer).
gz = Zlib.gzip(data)
puts "gzip:     #{gz.bytesize} bytes, restored? #{Zlib.gunzip(gz) == data}"

# Checksums over a byte string.
puts "crc32:    0x%08x" % Zlib.crc32("abc")
puts "adler32:  0x%08x" % Zlib.adler32("abc")

# Streaming API: feed data, then finish the stream.
deflater = Zlib::Deflate.new(Zlib::BEST_COMPRESSION)
stream = deflater.deflate(data, Zlib::FINISH) + deflater.finish
puts "stream:   #{deflater.total_out} bytes out, restored? #{Zlib::Inflate.inflate(stream) == data}"
