module HostRequirements where

open import Agda.Builtin.Unit
open import Data.String as String using (String)
open import Category.Applicative
open import Category.Monad
open import Category.Monad.State

data Path : Set where
  MkPath : String -> Path

data Value : Set where
  MkValue : String -> Value

data Identifier : Set where
  MkIdentifier : String -> Identifier

separator : String
separator = "/"

data Env : Set where
  MkEnv : Env

{-
get : Path -> State Env Value
get path = return (MkValue "")

set : Path -> Value -> State Env ⊤
set path value = return ⊤

delete : Path -> State Env ⊤
delete path = return ⊤

getConsensusState : State Env ConsensusState
getConsensusState = ?

getCallingModule : State Env Identifier
getCallingModule = return (MkIdentifier "")
-}
