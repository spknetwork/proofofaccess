# proof-of-access
Proof of Access is a go app that validates you are storing a file on ipfs

# Usage
Set your username in the .env file. Then run the following commands:

docker-compose build
docker run -p 3000:3000 --interactive --tty proofofaccess_app ./main -node 1 -username nathansmith
docker run --interactive --tty proofofaccess_app ./main -node 2 -username nathansenn

http://localhost:3000

enter your username and the CID hash you want to prove you have stored

# License
GNU General Public License v3.0

# Contact
nathan@d.buzz

# Contributors
https://github.com/nathansenn

# Funding
https://spk.network/ is funding this project.




