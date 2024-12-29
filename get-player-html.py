import requests
from bs4 import BeautifulSoup

# URL to scrape
url = "https://www.pro-football-reference.com/players/A/AaitIs00.htm"

# Fetch the HTML
response = requests.get(url)

if response.status_code != 200:
    print(f"Failed to fetch page, status code: {response.status_code}")
    exit()

# Print a snippet of the response content for debugging
print(f"Status Code: {response.status_code}")
print("Page Content Snippet:")
print(response.text[:10000])

# Parse the HTML
soup = BeautifulSoup(response.text, "html.parser")

# Find the first three player links
player_links = soup.select("div#players ul li a[href]")[:3]
print(f"Found {len(player_links)} player links.")

# Output each player's link
for i, player in enumerate(player_links, start=1):
    print(f"Player {i}: {player}")