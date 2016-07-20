# nomad

## this is 何？
一定時間ファイルサイズが変わっていないファイルを移動させたうえで元の一にシンボリックリンクを貼り直す

## インストール
```
% go get github.com/ieee0824/nomad
```

## 使い方
```
% nomad -src "${SOURCE_PATH}" -dst "${DESTINATION_PATH}"
```

# LICENSE
Written in Go and licensed under [the MIT License](https://opensource.org/licenses/MIT), it can also be used as a library.
