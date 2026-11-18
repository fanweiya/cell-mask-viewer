import os
import math
from PIL import Image, ImageDraw, ImageFilter

# 绘制高清晰度应用图标
size = 512
img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
draw = ImageDraw.Draw(img)

# 1. 绘制带有圆角的深色科技质感背景底板
pad = 24
radius = 110
bg_color = (15, 23, 42, 255)
border_color = (51, 65, 85, 255)

# 背景阴影
shadow = Image.new("RGBA", (size, size), (0, 0, 0, 0))
sdraw = ImageDraw.Draw(shadow)
sdraw.rounded_rectangle([pad + 4, pad + 12, size - pad + 4, size - pad + 12], radius=radius, fill=(0, 0, 0, 90))
shadow = shadow.filter(ImageFilter.GaussianBlur(16))
img = Image.alpha_composite(shadow, img)
draw = ImageDraw.Draw(img)

# 底板主体
draw.rounded_rectangle([pad, pad, size - pad, size - pad], radius=radius, fill=bg_color, outline=border_color, width=4)

# 2. 绘制显微镜十字微网格与同心圆指示环
center = size // 2
ring_color = (30, 41, 59, 200)
for r in [180, 120, 60]:
    draw.ellipse([center - r, center - r, center + r, center + r], outline=ring_color, width=2)
draw.line([center - 200, center, center + 200, center], fill=ring_color, width=2)
draw.line([center, center - 200, center, center + 200], fill=ring_color, width=2)

# 3. 绘制三个高反差实例细胞（带有半透明内部填充与高亮轮廓边缘）
cells_layer = Image.new("RGBA", (size, size), (0, 0, 0, 0))
cdraw = ImageDraw.Draw(cells_layer)

# 细胞 1：明亮翡翠绿（展示实例填充加高亮轮廓）
c1_box = [110, 120, 310, 320]
cdraw.ellipse(c1_box, fill=(16, 185, 129, 130), outline=(52, 211, 153, 255), width=8)
# 细胞 1 细胞核
cdraw.ellipse([180, 190, 240, 250], fill=(5, 150, 105, 200), outline=(110, 231, 183, 255), width=4)

# 细胞 2：电光青蓝
c2_box = [230, 180, 410, 370]
cdraw.ellipse(c2_box, fill=(14, 165, 233, 140), outline=(56, 189, 248, 255), width=8)
cdraw.ellipse([290, 250, 350, 310], fill=(2, 132, 199, 200), outline=(125, 211, 252, 255), width=4)

# 细胞 3：洋红玫瑰粉紫（只展示高光轮廓，体现“只看轮廓”模式）
c3_box = [150, 260, 350, 420]
cdraw.ellipse(c3_box, fill=(244, 63, 94, 60), outline=(251, 113, 133, 255), width=9)
cdraw.ellipse([220, 320, 280, 380], fill=(0, 0, 0, 0), outline=(253, 164, 175, 255), width=4)

# 合并细胞图层
img = Image.alpha_composite(img, cells_layer)

# 4. 生成不同分辨率的图标文件
os.makedirs("resources", exist_ok=True)
png_256 = img.resize((256, 256), Image.Resampling.LANCZOS)
png_256.save("resources/icon.png", format="PNG")
png_256.save("icon.png", format="PNG")

# 生成多尺寸包含 256, 128, 64, 48, 32, 16 的 ICO 图标文件
ico_sizes = [(256, 256), (128, 128), (64, 64), (48, 48), (32, 32), (16, 16)]
ico_imgs = [img.resize(s, Image.Resampling.LANCZOS) for s in ico_sizes]
ico_imgs[0].save("icon.ico", format="ICO", sizes=ico_sizes, append_images=ico_imgs[1:])
ico_imgs[0].save("resources/icon.ico", format="ICO", sizes=ico_sizes, append_images=ico_imgs[1:])
print("Icon generation completed successfully.")
