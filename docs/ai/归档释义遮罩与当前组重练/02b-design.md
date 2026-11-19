# 详细设计

阶段记录：GPT-6；2026-11-20 07:41 +08:00；@sw/dev-skills@2.0.0。授权与状态引用 01。

## 页面与组件

| 组件 | 输入/输出 | 职责 |
|---|---|---|
| WordCard | 保持 word、mode 和原事件 | archived 模式或 word.archived 时模糊中文；单卡按钮与文字点击揭示 |
| FlashcardView | useFlashcardQueue 新增 retryCount、restartGroup | 结果页有待加强词时显示“当前组再来一次”及数量，保留“再来一组” |
| useFlashcardQueue | 保持旧字段与方法，新增只读计数与重练动作 | 只在 finished 且非空、未加载/提交时筛选本轮 again/fuzzy，替换 cards 并重置 index、flipped、groupResults、error |

## 状态与交互

- WordCard 新增 meaningsRevealed=false，meaningsHidden 由归档身份和揭示状态派生；监听 id/归档状态/mode 变化复位。
- 中文主释义、例句译文、相关词释义标记为可遮罩目标，blur 作用于文字。word-body 捕获目标点击，隐藏时揭示并停止传播，防止触发原 gloss 标记。单独原生 button 提供键盘入口；隐藏内容 aria-hidden，揭示后恢复。英文查词和读音继续使用原逻辑。
- restartGroup 不请求服务器；筛选后保留单词对象与原顺序，清空当前轮结果，doneCount 不清零。每次后续评分照常调用已有 rate，最终产生新一轮结果；切页继续共享单例。
- 无待加强词时隐藏入口；方法也有守卫，首次点击进入新一轮后 finished=false，重复调用不改变队列。
- 项目使用 JavaScript，无新增 TS 类型；评分值仍为原字符串枚举。无接口或表结构变更。
