module Main where

import           Text.Pandoc
import           Text.Pandoc.Walk (walk)

behead :: Block -> Block
behead (Header n _ xs) | n >= 2 = Para [Emph xs]
behead x               = x

readDoc :: String -> Pandoc
readDoc s = readMarkdown def s
-- or, for pandoc 1.14 and greater, use:
-- readDoc s = case readMarkdown def s of
--                  Right doc -> doc
--                  Left err  -> error (show err)

writeDoc :: Pandoc -> String
writeDoc doc = writeMarkdown def doc

main :: IO ()
main = interact (writeDoc . walk behead . readDoc)
