# use with source
export GL_URL=github.com/rb3ckers
export GO_PROJECT_NAMESPACE=trafficmirror
if [ ! -d $(pwd)/.go ]
then
mkdir -p $(pwd)/.go/src/$GL_URL
ln -s $(pwd) $(pwd)/.go/src/$GL_URL/$GO_PROJECT_NAMESPACE
fi
export GOPATH=$(pwd)/.go
export GOBIN=$GOPATH/bin
export PATH=$GOBIN:$PATH
export GO_PROJECT_PATH=$(pwd)/.go/src/$GL_URL/$GO_PROJECT_NAMESPACE
