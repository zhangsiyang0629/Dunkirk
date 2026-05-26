import sys
from markitdown import MarkItDown
md = MarkItDown()
result = md.convert(sys.argv[1])
sys.stdout.write(result.text_content)