# proof-of-access
Proof of Access (PoA) is a mechanism used to validate that a storage node is storing a file on their local IPFS node. This technology is part of the SPK network and will be used to reward storage nodes with SPK cryptocurrency.

# Usage
To run the app locally, use the following command:
go run main.go -node 1 -username nathansmith
go run main.go -node 2 -username nathansenn

To run the app in Docker, you need to have Docker installed. Then run the following commands:

For docker, you will need to change `ipfs` and `pubsub` files to use:
var Shell = ipfs.NewShell("host.docker.internal:5001")

macOS:
docker-compose build
docker run -p 3000:3000 --interactive --tty proofofaccess_app ./main -node 1 -username nathansmith
docker run --interactive --tty proofofaccess_app ./main -node 2 -username nathansenn

Windows:
docker-compose build
docker run -p 3000:3000 --interactive --tty proofofaccess-app ./main -node 1 -username nathansmith
docker run --interactive --tty proofofaccess-app ./main -node 2 -username nathansenn

Once the app is running, go to http://localhost:3000 and enter your username and the CID hash you want to prove you have stored.

# License
GNU General Public License v3.0

# Contributors
Steven Ettinger (@disregardfiat)
https://github.com/disregardfiat
Nathan Senn (@nathansenn)
https://github.com/nathansenn

