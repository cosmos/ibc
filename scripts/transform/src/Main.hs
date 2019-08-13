module Main where

import           Data.Text
import           Data.Text.IO
import           Prelude            (error, id)
import           Protolude
import           System.Environment (getArgs)
import           Text.Pandoc
import           Text.Pandoc.Walk   (walk)

behead :: Block -> Block
behead x@(Header n t xs) | n == 2 =
  case t of
    ("Copyright", _, _) -> Null
    _                   -> x
behead x               = x

readDoc :: Text -> IO (Either PandocError Pandoc)
readDoc s = runIO (readMarkdown def s)

writeDoc :: Pandoc -> IO (Either PandocError Text)
writeDoc doc = runIO (writeMarkdown def doc)

main :: IO ()
main = do
  [fin, fout] <- getArgs
  print (fin, fout)
  Right doc <- readDoc =<< readFile fin
  let newDoc = walk (id :: Block -> Block) doc
  Right updated <- writeDoc newDoc
  writeFile fout updated
