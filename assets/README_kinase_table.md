# 激酶数据表 - 生成 Word 文档说明

本目录包含三张图合并后的激酶数据表，有两种方式得到 Word 文档：

## 方式一：用 HTML 转 Word（推荐，无需安装）

1. 用 **Microsoft Word** 直接打开 `kinase_table.html`
2. 菜单选择 **文件 → 另存为**
3. 保存类型选 **Word 文档 (*.docx)**，保存即可

表格内容与图片一致，可直接编辑。

## 方式二：用 Python 脚本生成 .docx

若已安装 Python，可生成标准 .docx 文件：

```bash
pip install python-docx
cd assets
python3 make_kinase_docx.py
```

会在当前目录生成 `kinase_table.docx`。
