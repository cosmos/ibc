module HostRequirements where

open import Data.String as String using (String)
open import Data.AVL.IndexedMap as Map

data Key : Set where
  MkKey : String -> Key

data Identifier : Set where
  MkIdentifier : String -> Identifier

separator : String
separator = "/"

data Env : Set where
  MkEnv : Map.Map' -> Env
