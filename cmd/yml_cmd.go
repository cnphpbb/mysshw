package cmd

// 专用库​：github.com/GuanceCloud/toml（BurntSushi/toml的分支，支持注释保留）
// 计划把原来SSHW项目的.sshw.yml文件迁移到mysshw项目的.toml文件
// 命令的格式
// mysshw yml -f <yml文件路径>
// 命令的功能
// 1. 读取<yml文件路径>文件
// 2. 把<yml文件路径>文件的内容解析成toml格式
// 3. 把解析后的toml内容追加到配置文件中
// 4. 提示用户配置文件已更新
// 5. 提示用户需要重启mysshw服务
// config example:
// # server group 1
// - name: server group 1
//   children:
//   - { name: server 1, user: root, host: 192.168.1.2 }
//   - { name: server 2, user: root, host: 192.168.1.3 }
//   - { name: server 3, user: root, host: 192.168.1.4 }

// # server group 2
// - name: server group 2
//   children:
//   - { name: server 1, user: root, host: 192.168.2.2 }
//   - { name: server 2, user: root, host: 192.168.3.3 }
//   - { name: dev server fully configured, user: appuser, host: 192.168.8.35, port: 22, password: 123456 }
//   - { name: dev server fully configured, user: appuser, host: 192.168.8.35, port: 22, password: 123456 }
//   - { name: dev server with key path, user: appuser, host: 192.168.8.35, port: 22, keypath: /root/.ssh/id_rsa }
//   - { name: dev server with passphrase key, user: appuser, host: 192.168.8.35, port: 22, keypath: /root/.ssh/id_rsa, passphrase: abcdefghijklmn}
//   - { name: dev server without port, user: appuser, host: 192.168.8.35 }
